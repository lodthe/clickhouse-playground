package qrunner

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

type localDockerRequestState struct {
	version string
	query   string

	chpImageName string
	containerID  string
}

type LocalDockerConfig struct {
	ExecRetryDelay time.Duration
	MaxExecRetries int

	// Path to the xml or yaml config which will be added to the ../config.d/ directory.
	CustomConfigPath *string
}

var DefaultLocalDockerConfig = LocalDockerConfig{
	ExecRetryDelay: 200 * time.Millisecond,
	MaxExecRetries: 20,

	CustomConfigPath: nil,
}

// LocalDocker executes SQL queries in docker containers
// that are created locally (Docker client engine API).
type LocalDocker struct {
	ctx context.Context
	cfg LocalDockerConfig

	repository string

	cli        *dockercli.Client
	tagStorage ImageTagStorage
}

func NewLocalDocker(ctx context.Context, cfg LocalDockerConfig, cli *dockercli.Client, repository string, tagStorage ImageTagStorage) *LocalDocker {
	return &LocalDocker{
		ctx:        ctx,
		cfg:        cfg,
		cli:        cli,
		repository: repository,
		tagStorage: tagStorage,
	}
}

// pull checks whether the requested image exists. If no, it will be downloaded and renamed to hashed-name.
func (r *LocalDocker) pull(ctx context.Context, state *localDockerRequestState) (err error) {
	startedAt := time.Now()
	imageName := FullImageName(r.repository, state.version)

	tag := r.tagStorage.Get(state.version)
	if tag == nil {
		return errors.New("version not found")
	}

	state.chpImageName = PlaygroundImageName(r.repository, tag.Digest)

	if r.checkIfImageExists(ctx, state) {
		return nil
	}

	out, err := r.cli.ImagePull(ctx, imageName, types.ImagePullOptions{})
	if err != nil {
		metrics.LocalDockerPipeline.PullNewImage(false, state.version, startedAt)
		return errors.Wrap(err, "docker pull failed")
	}

	output, err := io.ReadAll(out)
	if err != nil {
		zlog.Error().Err(err).Str("image", imageName).Msg("failed to read pull output")
	}

	zlog.Debug().Str("image", imageName).Str("output", string(output)).Msg("base image has been pulled")

	err = r.cli.ImageTag(ctx, imageName, state.chpImageName)
	if err != nil {
		metrics.LocalDockerPipeline.PullNewImage(false, state.version, startedAt)
		zlog.Error().Err(err).
			Str("source", imageName).
			Str("target", state.chpImageName).
			Msg("failed to rename image")

		return errors.Wrap(err, "failed to tag image")
	}

	metrics.LocalDockerPipeline.PullNewImage(true, state.version, startedAt)
	zlog.Debug().
		Dur("elapsed_ms", time.Since(startedAt)).
		Str("image", imageName).
		Msg("image has been pulled")

	return nil
}

func (r *LocalDocker) checkIfImageExists(ctx context.Context, state *localDockerRequestState) bool {
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
func (r *LocalDocker) runContainer(ctx context.Context, state *localDockerRequestState) (err error) {
	invokedAt := time.Now()
	defer func() {
		metrics.LocalDockerPipeline.CreateContainer(err == nil, state.version, invokedAt)
	}()

	contConfig := &container.Config{
		Image: state.chpImageName,
		Labels: map[string]string{
			"owner": "clickhouse-playground",
		},
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

	zlog.Debug().
		Dur("elapsed_ms", time.Since(invokedAt)).
		Str("image", state.chpImageName).
		Str("id", cont.ID).
		Msg("container has been created")
	createdAt := time.Now()

	err = r.cli.ContainerStart(ctx, cont.ID, types.ContainerStartOptions{})
	if err != nil {
		return errors.Wrap(err, "container cannot be started")
	}

	zlog.Debug().
		Dur("elapsed_ms", time.Since(createdAt)).
		Str("image", state.chpImageName).
		Str("id", cont.ID).
		Msg("container has been started")

	state.containerID = cont.ID

	return nil
}

func (r *LocalDocker) exec(ctx context.Context, state *localDockerRequestState) (stdout string, stderr string, err error) {
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

	zlog.Debug().
		Dur("elapsed_ms", time.Since(invokedAt)).
		Msg("exec finished")

	return outBuf.String(), errBuf.String(), nil
}

func (r *LocalDocker) runQuery(ctx context.Context, state *localDockerRequestState) (output string, err error) {
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

		if !strings.Contains(stderr, "DB::NetException: Connection refused") {
			zlog.Debug().
				Str("query", state.query).
				Str("stdout", stdout).
				Str("stderr", stderr).
				Msg("query has been executed")

			break
		}

		time.Sleep(r.cfg.ExecRetryDelay)
	}

	if stderr == "" {
		return stdout, nil
	}

	return stdout + "\n" + stderr, nil
}

func (r *LocalDocker) killContainer(state *localDockerRequestState) (err error) {
	invokedAt := time.Now()
	defer func() {
		metrics.LocalDockerPipeline.KillContainer(err == nil, state.version, invokedAt)
	}()

	err = r.cli.ContainerKill(r.ctx, state.containerID, "KILL")

	zlog.Debug().
		Dur("elapsed_ms", time.Since(invokedAt)).
		Str("image", state.chpImageName).
		Str("id", state.containerID).
		Msg("container has been killed")

	return err
}

func (r *LocalDocker) RunQuery(ctx context.Context, query string, version string) (string, error) {
	state := &localDockerRequestState{
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

		err = r.killContainer(state)
		if err != nil {
			zlog.Error().Err(err).Str("id", state.containerID).Msg("failed to kill container")
		}
	}()

	output, err := r.runQuery(ctx, state)
	if err != nil {
		return "", errors.Wrap(err, "failed to run query")
	}

	return output, nil
}
