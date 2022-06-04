package dockerengine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"sync"
	"time"

	"clickhouse-playground/internal/dockertag"
	"clickhouse-playground/internal/metrics"
	"clickhouse-playground/internal/qrunner"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	dockercli "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type ImageTagStorage interface {
	Get(version string) *dockertag.ImageTag
}

// Runner is a runner that creates database instances using Docker Engine API.
//
// This runner can start instances on arbitrary type of server, even on the same server where the coordinator
// is started. The main requirement is the running Docker daemon and granted access to it.
type Runner struct {
	ctx    context.Context
	cancel context.CancelFunc

	logger zerolog.Logger

	name string
	cfg  Config

	engine       *engineProvider
	tagStorage   ImageTagStorage
	pipelineMetr *metrics.PipelineExporter

	workers sync.WaitGroup
	gc      *garbageCollector
	status  *statusCollector
}

func New(ctx context.Context, logger zerolog.Logger, name string, cfg Config, tagStorage ImageTagStorage) (*Runner, error) {
	engine, err := newProvider(ctx, cfg.DaemonURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Docker engine provider")
	}

	ctx, cancel := context.WithCancel(ctx)

	logger = logger.With().Str("runner", name).Logger()
	gc := newGarbageCollector(ctx, logger, cfg.GC, cfg.Repository, engine, metrics.NewRunnerGCExporter(string(qrunner.TypeDockerEngine), name))
	status := newStatusCollector(ctx, logger, cfg.Repository, cfg.StatusCollectionFrequency, engine, metrics.NewRunnerStatusExporter(string(qrunner.TypeDockerEngine), name))

	return &Runner{
		ctx:          ctx,
		cancel:       cancel,
		logger:       logger,
		name:         name,
		cfg:          cfg,
		engine:       engine,
		tagStorage:   tagStorage,
		pipelineMetr: metrics.NewPipelineExporter(string(qrunner.TypeDockerEngine), name),
		gc:           gc,
		status:       status,
	}, nil
}

func (r *Runner) Type() qrunner.Type {
	return qrunner.TypeDockerEngine
}

func (r *Runner) Name() string {
	return r.name
}

// Start runs the following background tasks:
// 1) gc -- prunes containers and images;
// 2) status exporter -- exports information about current state of the runner.
func (r *Runner) Start() error {
	r.workers.Add(1)
	go func() {
		defer r.workers.Done()
		r.gc.start()
	}()

	r.workers.Add(1)
	go func() {
		defer r.workers.Done()
		r.status.start()
	}()

	logCtx := r.logger.Info()
	if r.cfg.DaemonURL != nil {
		logCtx = logCtx.Str("daemon_url", *r.cfg.DaemonURL)
	}
	logCtx.Msg("runner has been started")

	return nil
}

func (r *Runner) Stop() error {
	r.logger.Info().Msg("stopping")

	r.cancel()
	r.workers.Wait()

	r.logger.Info().Msg("runner has been stopped")

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

		startedAt := time.Now()
		defer func() {
			r.pipelineMetr.RemoveContainer(err == nil, "", startedAt)
		}()

		err = r.engine.removeContainer(r.ctx, state.containerID, true)
		if err != nil {
			r.logger.Error().Err(err).Str("run_id", state.runID).Msg("failed to kill container")
		}

		r.logger.Debug().Str("container_id", state.containerID).Msg("container has been force removed")
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

	out, err := r.engine.pullImage(ctx, imageName)
	if err != nil {
		r.pipelineMetr.PullNewImage(false, state.version, startedAt)
		return errors.Wrap(err, "docker pull failed")
	}

	// We should read the output to be sure that the image has been pulled.
	_, err = io.ReadAll(out)
	if err != nil {
		r.logger.Error().Err(err).Str("image", imageName).Msg("failed to read pull output")
	}

	r.logger.Debug().Str("image", imageName).Msg("base image has been pulled")

	err = r.engine.addImageTag(ctx, imageName, state.chpImageName)
	if err != nil {
		r.pipelineMetr.PullNewImage(false, state.version, startedAt)
		r.logger.Error().Err(err).
			Str("run_id", state.runID).
			Str("source", imageName).
			Str("target", state.chpImageName).
			Msg("failed to rename image")

		return errors.Wrap(err, "failed to tag image")
	}

	r.pipelineMetr.PullNewImage(true, state.version, startedAt)
	r.logger.Debug().
		Str("run_id", state.runID).
		Dur("elapsed_ms", time.Since(startedAt)).
		Str("image", imageName).
		Msg("image has been pulled")

	return nil
}

func (r *Runner) checkIfImageExists(ctx context.Context, state *requestState) bool {
	startedAt := time.Now()

	_, err := r.engine.getImageByID(ctx, state.chpImageName)
	if err == nil {
		r.pipelineMetr.PullExistedImage(true, state.version, startedAt)
		r.logger.Debug().
			Dur("elapsed_ms", time.Since(startedAt)).
			Str("image", state.chpImageName).
			Msg("image has already been pulled")

		return true
	}
	if err != nil && !dockercli.IsErrNotFound(err) {
		r.pipelineMetr.PullExistedImage(false, state.version, startedAt)
		r.logger.Error().Err(err).Str("image", state.chpImageName).Msg("docker inspect failed")
	}

	return false
}

// runContainer starts a container and returns its id.
func (r *Runner) runContainer(ctx context.Context, state *requestState) (err error) {
	invokedAt := time.Now()
	defer func() {
		r.pipelineMetr.CreateContainer(err == nil, state.version, invokedAt)
	}()

	contConfig := &container.Config{
		Image:  state.chpImageName,
		Labels: qrunner.CreateContainerLabels(state.runID, state.version),
	}

	hostConfig := &container.HostConfig{
		Resources: container.Resources{
			NanoCPUs:   int64(r.cfg.Container.CPULimit),
			CpusetCpus: r.cfg.Container.CPUSet,
			Memory:     int64(r.cfg.Container.MemoryLimit),
		},
	}

	// A custom config is used to disable some ClickHouse features to speed up the startup.
	if r.cfg.CustomConfigPath != nil {
		hostConfig.Mounts = append(hostConfig.Mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   *r.cfg.CustomConfigPath,
			Target:   fmt.Sprintf("/etc/clickhouse-server/config.d/custom-config%s", path.Ext(*r.cfg.CustomConfigPath)),
			ReadOnly: true,
		})
	}

	cont, err := r.engine.createContainer(ctx, contConfig, hostConfig)
	if err != nil {
		return errors.Wrap(err, "container cannot be created")
	}

	createdAt := time.Now()
	debugLogger := r.logger.Debug().
		Str("run_id", state.runID).
		Str("image", state.chpImageName).
		Str("container_id", cont.ID)
	debugLogger.Dur("elapsed_ms", time.Since(invokedAt)).Msg("container has been created")

	err = r.engine.startContainer(ctx, cont.ID)
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
		r.pipelineMetr.ExecCommand(err == nil, state.version, invokedAt)
	}()

	resp, err := r.engine.exec(ctx, state.containerID, []string{"clickhouse-client", "-n", "-m", "--query", state.query})
	if err != nil {
		return "", "", errors.Wrap(err, "exec failed")
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

	r.logger.Debug().Str("run_id", state.runID).Dur("elapsed_ms", time.Since(invokedAt)).Msg("exec finished")

	return outBuf.String(), errBuf.String(), nil
}

func (r *Runner) runQuery(ctx context.Context, state *requestState) (output string, err error) {
	invokedAt := time.Now()
	defer func() {
		r.pipelineMetr.RunQuery(err == nil, state.version, invokedAt)
	}()

	var stdout string
	var stderr string

	for retry := 0; retry < r.cfg.MaxExecRetries; retry++ {
		stdout, stderr, err = r.exec(ctx, state)
		if err != nil {
			return "", err
		}

		if qrunner.CheckIfClickHouseIsReady(stderr) {
			r.logger.Debug().Str("run_id", state.runID).Msg("query has been executed")
			break
		}

		time.Sleep(r.cfg.ExecRetryDelay)
	}

	if stderr == "" {
		return stdout, nil
	}

	return stdout + "\n" + stderr, nil
}
