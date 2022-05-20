package localdocker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"clickhouse-playground/internal/dockertag"
	"clickhouse-playground/internal/metrics"
	"clickhouse-playground/internal/qrunner"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	dockercli "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/pkg/errors"
	zlog "github.com/rs/zerolog/log"
)

type ImageTagStorage interface {
	Get(version string) *dockertag.ImageTag
}

// Runner executes SQL queries in docker containers
// that are created locally (Docker client engine API).
type Runner struct {
	ctx context.Context
	cfg Config

	repository string

	cli        *dockercli.Client
	tagStorage ImageTagStorage
}

func New(ctx context.Context, cfg Config, cli *dockercli.Client, repository string, tagStorage ImageTagStorage) *Runner {
	return &Runner{
		ctx:        ctx,
		cfg:        cfg,
		cli:        cli,
		repository: repository,
		tagStorage: tagStorage,
	}
}

func (r *Runner) isStopped() bool {
	select {
	case <-r.ctx.Done():
		return true

	default:
		return false
	}
}

// StartGarbageCollector triggers periodically the garbage collector
// to prune infrequently used images and hanged up containers.
// Trigger frequency, image and container TTL, and other gc are configured in Config.
func (r *Runner) StartGarbageCollector() {
	cfg := r.cfg.GC
	if cfg == nil {
		zlog.Info().Msg("localdocker gc is disabled due to a missed configuration")
		return
	}

	zlog.Info().Dur("trigger_frequency", cfg.TriggerFrequency).Msg("localdocker gc has been started")
	defer zlog.Info().Msg("localdocker gc has been finished")

	trigger := func() {
		err := r.triggerGC()
		if err != nil {
			zlog.Err(err).Msg("localdocker gc trigger failed")
		}
	}

	trigger()

	t := time.NewTicker(cfg.TriggerFrequency)

	for {
		select {
		case <-r.ctx.Done():
			return

		case <-t.C:
		}

		trigger()
	}
}

func (r *Runner) triggerGC() (err error) {
	if r.isStopped() {
		return nil
	}

	startedAt := time.Now()

	err = r.triggerContainersGC()
	if err != nil {
		return errors.Wrap(err, "containers gc failed")
	}

	if r.isStopped() {
		return nil
	}

	err = r.triggerImagesGC()
	if err != nil {
		return errors.Wrap(err, "images gc failed")
	}

	zlog.Info().Dur("elapsed_ms", time.Since(startedAt)).Msg("gc finished")

	return nil
}

func (r *Runner) triggerContainersGC() error {
	pruneOut, err := r.cli.ContainersPrune(r.ctx, filters.NewArgs(filters.Arg("label", qrunner.LabelOwnership)))
	if err != nil {
		return errors.Wrap(err, "failed to prune stopped containers")
	}

	zlog.Info().
		Uint64("space_reclaimed_bytes", pruneOut.SpaceReclaimed).
		Int("container_count", len(pruneOut.ContainersDeleted)).
		Msg("stopped containers have been deleted")

	if r.cfg.GC.ContainerTTL == nil {
		return nil
	}

	// Find hanged up containers and force remove them.
	containers, err := r.cli.ContainerList(r.ctx, types.ContainerListOptions{
		Size:    true,
		All:     true,
		Limit:   -1,
		Filters: filters.NewArgs(filters.Arg("label", qrunner.LabelOwnership)),
	})
	if err != nil {
		return errors.Wrap(err, "failed to list containers")
	}

	var count uint
	var reclaimedSpace int64
	for _, c := range containers {
		deadline := time.Unix(c.Created, 0).Add(*r.cfg.GC.ContainerTTL)
		if time.Now().Before(deadline) {
			continue
		}

		err = r.forceRemoveContainer(c.ID)
		if err != nil {
			zlog.Error().Err(err).Str("container_id", c.ID).Msg("containers gc failed to remove container")
			continue
		}

		count++
		reclaimedSpace += c.SizeRw
	}

	zlog.Info().
		Int64("space_reclaimed_bytes", reclaimedSpace).
		Uint("container_count", count).
		Msg("hanged up containers have been stopped")

	return nil
}

func (r *Runner) triggerImagesGC() error {
	// Not implemented.
	return nil
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

		err = r.forceRemoveContainer(state.containerID)
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
	imageName := qrunner.FullImageName(r.repository, state.version)

	tag := r.tagStorage.Get(state.version)
	if tag == nil {
		return errors.New("version not found")
	}

	state.chpImageName = qrunner.PlaygroundImageName(r.repository, tag.Digest)

	if r.checkIfImageExists(ctx, state) {
		return nil
	}

	out, err := r.cli.ImagePull(ctx, imageName, types.ImagePullOptions{})
	if err != nil {
		metrics.LocalDockerPipeline.PullNewImage(false, state.version, startedAt)
		return errors.Wrap(err, "docker pull failed")
	}

	// We should read the output to be sure that the image has been pulled.
	output, err := io.ReadAll(out)
	if err != nil {
		zlog.Error().Err(err).Str("image", imageName).Msg("failed to read pull output")
	}

	zlog.Trace().Str("image", imageName).Str("output", string(output)).Msg("base image has been pulled")

	err = r.cli.ImageTag(ctx, imageName, state.chpImageName)
	if err != nil {
		metrics.LocalDockerPipeline.PullNewImage(false, state.version, startedAt)
		zlog.Error().Err(err).
			Str("run_id", state.runID).
			Str("source", imageName).
			Str("target", state.chpImageName).
			Msg("failed to rename image")

		return errors.Wrap(err, "failed to tag image")
	}

	metrics.LocalDockerPipeline.PullNewImage(true, state.version, startedAt)
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
		metrics.LocalDockerPipeline.PullExistedImage(true, state.version, startedAt)
		zlog.Debug().
			Dur("elapsed_ms", time.Since(startedAt)).
			Str("image", state.chpImageName).
			Msg("image has already been pulled")

		return true
	}
	if err != nil && !dockercli.IsErrNotFound(err) {
		metrics.LocalDockerPipeline.PullExistedImage(false, state.version, startedAt)
		zlog.Error().Err(err).Str("image", state.chpImageName).Msg("docker inspect failed")
	}

	return false
}

// runContainer starts a container and returns its id.
func (r *Runner) runContainer(ctx context.Context, state *requestState) (err error) {
	invokedAt := time.Now()
	defer func() {
		metrics.LocalDockerPipeline.CreateContainer(err == nil, state.version, invokedAt)
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
		metrics.LocalDockerPipeline.ExecCommand(err == nil, state.version, invokedAt)
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
		metrics.LocalDockerPipeline.RunQuery(err == nil, state.version, invokedAt)
	}()

	var stdout string
	var stderr string

	for retry := 0; retry < r.cfg.MaxExecRetries; retry++ {
		stdout, stderr, err = r.exec(ctx, state)
		if err != nil {
			return "", err
		}

		if r.checkIfQueryExecuted(stdout, stderr) {
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

// checkIfQueryExecuted checks whether a clickhouse instance has accepted a query.
// We have no mechanism to be signaled when a clickhouse instance is ready to accept queries,
// so we try to send them continuously until the instance accepts them.
// When the instance is not ready, we received the 'Connection refused' exception.
func (r *Runner) checkIfQueryExecuted(_, stderr string) bool {
	return !strings.Contains(stderr, "DB::NetException: Connection refused")
}

func (r *Runner) forceRemoveContainer(id string) (err error) {
	invokedAt := time.Now()
	defer func() {
		metrics.LocalDockerPipeline.RemoveContainer(err == nil, "", invokedAt)
	}()

	err = r.cli.ContainerRemove(r.ctx, id, types.ContainerRemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	})
	if err != nil {
		return err
	}

	zlog.Trace().Str("container_id", id).Msg("container has been force removed")

	return nil
}
