package dockerengine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"time"

	"clickhouse-playground/internal/dockertag"
	"clickhouse-playground/internal/metrics"
	"clickhouse-playground/internal/qrunner"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	dockercli "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/pkg/errors"
	zlog "github.com/rs/zerolog/log"
)

type ImageTagStorage interface {
	Get(version string) *dockertag.ImageTag
}

// Runner is a runner that creates database instances using Docker Engine API.
//
// This runner can start instances on arbitrary type of server, even on the same server where the coordinator
// is started. The main requirement is the running Docker daemon and granted access to it.
type Runner struct {
	ctx  context.Context
	name string
	cfg  Config

	cli        *dockercli.Client
	tagStorage ImageTagStorage
	gc         *garbageCollector
}

func New(ctx context.Context, name string, cfg Config, cli *dockercli.Client, tagStorage ImageTagStorage) *Runner {
	return &Runner{
		ctx:        ctx,
		name:       name,
		cfg:        cfg,
		cli:        cli,
		tagStorage: tagStorage,
		gc:         newGarbageCollector(ctx, cfg.GC, cfg.Repository, cli),
	}
}

func (r *Runner) Type() qrunner.Type {
	return qrunner.TypeEC2
}

func (r *Runner) Name() string {
	return r.name
}

// StartGarbageCollector triggers periodically the garbage collector
// to prune infrequently used images and hanged up containers.
// Trigger frequency, image and container TTL, and other gc are configured in Config.
func (r *Runner) StartGarbageCollector() {
	r.gc.start()
}

func (r *Runner) RunQuery(ctx context.Context, runID string, query string, version string) (string, error) {
	state := &requestState{
		runID:   runID,
		version: version,
		query:   query,
	}

	err := r.pull(ctx, state)
	if err != nil {
		return "", errors.Wrap(err, "pull failed")
	}

	err = r.runContainer(ctx, state)
	if err != nil {
		return "", errors.Wrap(err, "failed to run container")
	}

	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
		case <-done:
		}

		startedAt := time.Now()
		defer func() {
			metrics.DockerEnginePipeline.RemoveContainer(err == nil, "", startedAt)
		}()

		err = r.gc.forceRemoveContainer(state.containerID)
		if err != nil {
			zlog.Error().Err(err).Str("run_id", state.runID).Msg("failed to kill container")
		}
	}()

	output, err := r.runQuery(ctx, state)
	if err != nil {
		return "", errors.Wrap(err, "failed to run query")
	}

	return output, nil
}

// pull checks whether the requested image exists. If no, it will be downloaded and renamed to hashed-name.
func (r *Runner) pull(ctx context.Context, state *requestState) (err error) {
	startedAt := time.Now()
	imageName := qrunner.FullImageName(r.cfg.Repository, state.version)

	tag := r.tagStorage.Get(state.version)
	if tag == nil {
		return errors.New("version not found")
	}

	state.chpImageName = qrunner.PlaygroundImageName(r.cfg.Repository, tag.Digest)

	if r.checkIfImageExists(ctx, state) {
		return nil
	}

	out, err := r.cli.ImagePull(ctx, imageName, types.ImagePullOptions{})
	if err != nil {
		metrics.DockerEnginePipeline.PullNewImage(false, state.version, startedAt)
		return errors.Wrap(err, "docker pull failed")
	}

	// We should read the output to be sure that the image has been pulled.
	output, err := io.ReadAll(out)
	if err != nil {
		zlog.Error().Err(err).Str("image", imageName).Msg("failed to read pull output")
	}

	zlog.Debug().Str("image", imageName).Str("output", string(output)).Msg("base image has been pulled")

	err = r.cli.ImageTag(ctx, imageName, state.chpImageName)
	if err != nil {
		metrics.DockerEnginePipeline.PullNewImage(false, state.version, startedAt)
		zlog.Error().Err(err).
			Str("run_id", state.runID).
			Str("source", imageName).
			Str("target", state.chpImageName).
			Msg("failed to rename image")

		return errors.Wrap(err, "failed to tag image")
	}

	metrics.DockerEnginePipeline.PullNewImage(true, state.version, startedAt)
	zlog.Debug().
		Str("run_id", state.runID).
		Dur("elapsed_ms", time.Since(startedAt)).
		Str("image", imageName).
		Msg("image has been pulled")

	return nil
}

func (r *Runner) checkIfImageExists(ctx context.Context, state *requestState) bool {
	startedAt := time.Now()

	_, _, err := r.cli.ImageInspectWithRaw(ctx, state.chpImageName)
	if err == nil {
		metrics.DockerEnginePipeline.PullExistedImage(true, state.version, startedAt)
		zlog.Debug().
			Dur("elapsed_ms", time.Since(startedAt)).
			Str("image", state.chpImageName).
			Msg("image has already been pulled")

		return true
	}
	if err != nil && !dockercli.IsErrNotFound(err) {
		metrics.DockerEnginePipeline.PullExistedImage(false, state.version, startedAt)
		zlog.Error().Err(err).Str("image", state.chpImageName).Msg("docker inspect failed")
	}

	return false
}

// runContainer starts a container and returns its id.
func (r *Runner) runContainer(ctx context.Context, state *requestState) (err error) {
	invokedAt := time.Now()
	defer func() {
		metrics.DockerEnginePipeline.CreateContainer(err == nil, state.version, invokedAt)
	}()

	contConfig := &container.Config{
		Image:  state.chpImageName,
		Labels: qrunner.CreateContainerLabels(state.runID, state.version),
	}

	hostConfig := new(container.HostConfig)

	// A custom config is used to disable some ClickHouse features to speed up the startup.
	if r.cfg.CustomConfigPath != nil {
		hostConfig.Mounts = append(hostConfig.Mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   *r.cfg.CustomConfigPath,
			Target:   fmt.Sprintf("/etc/clickhouse-server/config.d/custom-config%s", path.Ext(*r.cfg.CustomConfigPath)),
			ReadOnly: true,
		})
	}

	cont, err := r.cli.ContainerCreate(ctx, contConfig, hostConfig, nil, nil, "")
	if err != nil {
		return errors.Wrap(err, "container cannot be created")
	}

	createdAt := time.Now()
	debugLogger := zlog.Debug().
		Str("run_id", state.runID).
		Str("image", state.chpImageName).
		Str("container_id", cont.ID)
	debugLogger.Dur("elapsed_ms", time.Since(invokedAt)).Msg("container has been created")

	err = r.cli.ContainerStart(ctx, cont.ID, types.ContainerStartOptions{})
	if err != nil {
		return errors.Wrap(err, "container cannot be started")
	}

	debugLogger.Dur("elapsed_ms", time.Since(createdAt)).Msg("container has been started")

	state.containerID = cont.ID

	return nil
}

func (r *Runner) exec(ctx context.Context, state *requestState) (stdout string, stderr string, err error) {
	invokedAt := time.Now()
	defer func() {
		metrics.DockerEnginePipeline.ExecCommand(err == nil, state.version, invokedAt)
	}()

	exec, err := r.cli.ContainerExecCreate(ctx, state.containerID, types.ExecConfig{
		AttachStderr: true,
		AttachStdout: true,
		Cmd:          []string{"clickhouse-client", "-n", "-m", "--query", state.query},
	})
	if err != nil {
		return "", "", errors.Wrap(err, "exec create failed")
	}

	resp, err := r.cli.ContainerExecAttach(ctx, exec.ID, types.ExecStartCheck{})
	if err != nil {
		return "", "", errors.Wrap(err, "exec attach failed")
	}
	defer resp.Close()

	// https://github.com/moby/moby/blob/8e610b2b55bfd1bfa9436ab110d311f5e8a74dcb/integration/internal/container/exec.go#L38
	var outBuf, errBuf bytes.Buffer
	outputDone := make(chan error)

	go func() {
		_, err = stdcopy.StdCopy(&outBuf, &errBuf, resp.Reader)
		outputDone <- err
	}()

	select {
	case err := <-outputDone:
		if err != nil {
			return "", "", errors.Wrap(err, "failed to get output")
		}

	case <-ctx.Done():
		return "", "", ctx.Err()
	}

	zlog.Debug().Str("run_id", state.runID).Dur("elapsed_ms", time.Since(invokedAt)).Msg("exec finished")

	return outBuf.String(), errBuf.String(), nil
}

func (r *Runner) runQuery(ctx context.Context, state *requestState) (output string, err error) {
	invokedAt := time.Now()
	defer func() {
		metrics.DockerEnginePipeline.RunQuery(err == nil, state.version, invokedAt)
	}()

	var stdout string
	var stderr string

	for retry := 0; retry < r.cfg.MaxExecRetries; retry++ {
		stdout, stderr, err = r.exec(ctx, state)
		if err != nil {
			return "", err
		}

		if qrunner.CheckIfClickHouseIsReady(stderr) {
			zlog.Debug().Str("run_id", state.runID).Msg("query has been executed")
			break
		}

		time.Sleep(r.cfg.ExecRetryDelay)
	}

	if stderr == "" {
		return stdout, nil
	}

	return stdout + "\n" + stderr, nil
}
