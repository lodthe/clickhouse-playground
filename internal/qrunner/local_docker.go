package qrunner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"clickhouse-playground/internal/dockertag"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockercli "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"
	zlog "github.com/rs/zerolog/log"
)

type ImageTagStorage interface {
	Get(version string) *dockertag.ImageTag
}

// LocalDocker executes SQL queries in docker containers
// that are created locally (Docker client engine API).
type LocalDocker struct {
	ctx context.Context
	cli *dockercli.Client

	repository string

	tagStorage ImageTagStorage
}

func NewLocalDocker(ctx context.Context, cli *dockercli.Client, repository string, tagStorage ImageTagStorage) *LocalDocker {
	return &LocalDocker{
		ctx:        ctx,
		cli:        cli,
		repository: repository,
		tagStorage: tagStorage,
	}
}

func (r *LocalDocker) pull(ctx context.Context, version string) error {
	startedAt := time.Now()
	imageName := FullImageName(r.repository, version)

	tag := r.tagStorage.Get(version)
	if tag == nil {
		return errors.New("version not found")
	}

	chpImageName := PlaygroundImageName(r.repository, tag.Digest)

	_, _, err := r.cli.ImageInspectWithRaw(ctx, chpImageName)
	if err == nil {
		zlog.Debug().
			Dur("elapsed_ms", time.Since(startedAt)).
			Str("image", chpImageName).
			Msg("the image has already been pulled")

		return nil
	}
	if err != nil && !dockercli.IsErrNotFound(err) {
		zlog.Error().Err(err).Str("image", imageName).Msg("docker inspect failed")
	}

	out, err := r.cli.ImagePull(ctx, imageName, types.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer out.Close()

	err = r.cli.ImageTag(ctx, imageName, chpImageName)
	if err != nil {
		zlog.Error().Err(err).
			Str("source", imageName).
			Str("target", chpImageName).
			Msg("failed to rename image")

		return errors.Wrap(err, "failed to tag image")
	}

	_, err = io.ReadAll(out)
	if err != nil {
		return errors.Wrap(err, "failed to read pull output")
	}

	zlog.Debug().
		Dur("elapsed_ms", time.Since(startedAt)).
		Str("image", imageName).
		Msg("image has been pulled")

	return nil
}

// runContainer starts a container and returns its id.
func (r *LocalDocker) runContainer(ctx context.Context, clickhouseVersion string) (id string, hostPort string, err error) {
	err = r.pull(ctx, clickhouseVersion)
	if err != nil {
		return "", "", errors.Wrap(err, "pull failed")
	}

	pulledAt := time.Now()

	const httpInterfacePort = nat.Port("8123/tcp")
	hostPort = strconv.Itoa(50000 + rand.Intn(400)) // nolint:gosec

	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			httpInterfacePort: []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: hostPort,
				},
			},
		},
	}

	contConfig := &container.Config{
		Image: fmt.Sprintf("%s:%s", r.repository, clickhouseVersion),
		Labels: map[string]string{
			"clickhouse-playground": "true",
		},
		ExposedPorts: nat.PortSet{
			httpInterfacePort: {},
		},
	}

	cont, err := r.cli.ContainerCreate(ctx, contConfig, hostConfig, nil, nil, "")
	if err != nil {
		return "", "", errors.Wrap(err, "container cannot be created")
	}

	zlog.Debug().
		Dur("elapsed_ms", time.Since(pulledAt)).
		Str("version", clickhouseVersion).
		Str("id", cont.ID).
		Msg("container has been created")
	createdAt := time.Now()

	err = r.cli.ContainerStart(ctx, cont.ID, types.ContainerStartOptions{})
	if err != nil {
		return "", "", errors.Wrap(err, "container cannot be started")
	}

	if err != nil {
		return
	}

	zlog.Debug().
		Dur("elapsed_ms", time.Since(createdAt)).
		Str("version", clickhouseVersion).
		Str("id", cont.ID).
		Msg("container has been started")

	return cont.ID, hostPort, nil
}

func (r *LocalDocker) killContainer(ctx context.Context, id string) error {
	invokedAt := time.Now()
	err := r.cli.ContainerKill(ctx, id, "KILL")

	zlog.Debug().
		Dur("elapsed_ms", time.Since(invokedAt)).
		Str("id", id).
		Msg("container has been killed")

	return err
}

func (r *LocalDocker) exec(ctx context.Context, containerID string, query string) (stdout string, stderr string, err error) {
	invokedAt := time.Now()
	exec, err := r.cli.ContainerExecCreate(ctx, containerID, types.ExecConfig{
		AttachStderr: true,
		AttachStdout: true,
		Cmd:          []string{"clickhouse-client", "-n", "-m", "--query", query},
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

func (r *LocalDocker) runQuery(ctx context.Context, containerID string, query string) (string, error) {
	var stdout string
	var stderr string
	var err error

	for retry := 0; retry < 15; retry++ {
		stdout, stderr, err = r.exec(ctx, containerID, query)
		if err != nil {
			return "", err
		}

		if !strings.Contains(stderr, "DB::NetException: Connection refused") {
			zlog.Debug().
				Str("query", query).
				Str("stdout", stdout).
				Str("stderr", stderr).
				Msg("a query has been executed")

			break
		}

		time.Sleep(300 * time.Millisecond)
	}

	if stderr == "" {
		return stdout, nil
	}

	return stdout + "\n" + stderr, nil
}

func (r *LocalDocker) RunQuery(ctx context.Context, query string, version string) (string, error) {
	containerID, hostPort, err := r.runContainer(ctx, version)
	if err != nil {
		return "", errors.Wrap(err, "failed to run container")
	}

	defer func() {
		err = r.killContainer(ctx, containerID)
		if err != nil {
			zlog.Error().Err(err).Str("id", containerID).Msg("failed to kill container")
		}
	}()

	addr := fmt.Sprintf("http://localhost:%s", hostPort)
	_ = addr
	output, err := r.runQuery(ctx, containerID, query)
	if err != nil {
		return "", errors.Wrap(err, "failed to run query")
	}

	return output, nil
}
