package dockerengine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"sync"
	"time"

	"clickhouse-playground/internal/database"
	"clickhouse-playground/internal/database/runsettings"
	"clickhouse-playground/internal/dockertag"
	"clickhouse-playground/internal/metrics"
	"clickhouse-playground/internal/qrunner"
	"clickhouse-playground/internal/queryrun"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	dockercli "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type ImageStorage interface {
	Find(version string) (dockertag.Image, bool)
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
	tagStorage   ImageStorage
	pipelineMetr *metrics.PipelineExporter

	workers   sync.WaitGroup
	gc        *garbageCollector
	status    *statusCollector
	prewarmer *prewarmer
}

func New(ctx context.Context, logger zerolog.Logger, name string, cfg Config, tagStorage ImageStorage) (*Runner, error) {
	engine, err := newProvider(ctx, cfg.DaemonURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Docker engine provider")
	}

	ctx, cancel := context.WithCancel(ctx)

	logger = logger.With().Str("runner", name).Logger()

	runner := &Runner{
		ctx:          ctx,
		cancel:       cancel,
		logger:       logger,
		name:         name,
		cfg:          cfg,
		engine:       engine,
		tagStorage:   tagStorage,
		pipelineMetr: metrics.NewPipelineExporter(string(qrunner.TypeDockerEngine), name),
	}

	runner.gc = newGarbageCollector(ctx, logger, cfg.GC, engine, metrics.NewRunnerGCExporter(string(qrunner.TypeDockerEngine), name))
	runner.status = newStatusCollector(ctx, logger, cfg.StatusCollectionFrequency, engine, metrics.NewRunnerStatusExporter(string(qrunner.TypeDockerEngine), name))
	runner.prewarmer = newPrewarmer(ctx, logger, runner, runner.engine, cfg.MaxWarmContainers)

	return runner, nil
}

func (r *Runner) Type() qrunner.Type {
	return qrunner.TypeDockerEngine
}

func (r *Runner) Name() string {
	return r.name
}

func (r *Runner) Status(ctx context.Context) qrunner.RunnerStatus {
	err := r.engine.ping(ctx)

	return qrunner.RunnerStatus{
		Alive:            err == nil,
		LivenessProbeErr: err,
	}
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

	r.workers.Add(1)
	go func() {
		defer r.workers.Done()
		r.prewarmer.Start()
	}()

	logCtx := r.logger.Info()
	if r.cfg.DaemonURL != nil {
		logCtx = logCtx.Str("daemon_url", *r.cfg.DaemonURL)
	}
	logCtx.Msg("runner has been started")

	return nil
}

func (r *Runner) Stop(shutdownCtx context.Context) error {
	r.logger.Info().Msg("stopping")

	r.prewarmer.Stop(shutdownCtx)

	r.cancel()
	r.workers.Wait()

	r.logger.Info().Msg("runner has been stopped")

	return nil
}

func (r *Runner) RunQuery(ctx context.Context, run *queryrun.Run) (output string, err error) {
	state := &requestState{
		runID:    run.ID,
		database: run.Database,
		version:  run.Version,
		query:    run.Input,
		settings: run.Settings,
	}

	state.imageTag, state.imageFQN, err = r.constructImageFQN(state.version)
	if err != nil {
		return "", fmt.Errorf("failed to construct FQN: %w", err)
	}

	containerID, found, err := r.prewarmer.Fetch(state.imageFQN)
	if err != nil {
		r.logger.Err(err).Str("run_id", state.runID).Msg("failed to fetch a prewarmed container")
	}
	if found {
		state.containerID = containerID
	} else {
		err := r.createContainer(ctx, state)
		if err != nil {
			return "", fmt.Errorf("failed to create container: %w", err)
		}
	}

	r.prewarmer.PushNewRequest(*state)

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

		err := r.engine.removeContainer(r.ctx, state.containerID)
		if err != nil {
			r.logger.Error().Err(err).Str("run_id", state.runID).Msg("failed to kill container")
			return
		}

		r.logger.Debug().Str("container_id", state.containerID).Msg("container has been force removed")
	}()

	output, err = r.runQuery(ctx, state)
	if err != nil {
		return "", errors.Wrap(err, "failed to run query")
	}

	return output, nil
}

// constructImageFQN builds image tag and FQN from version.
// If there is no such a version, an error is returned.
//
// Otherwise, an image is fetched and the following names are built:
// - image tag: image name in format <repository>:<version>
// - image FQN: a unique fully qualified name that includes the exact version of the image
func (r *Runner) constructImageFQN(version string) (imageTag string, imageFQN string, err error) {
	img, found := r.tagStorage.Find(version)
	if !found {
		return "", "", errors.New("version not found")
	}

	imageTag = qrunner.FullImageName(img.Repository, version)
	imageFQN = qrunner.PlaygroundImageName(img.Repository, img.Digest)

	return imageTag, imageFQN, nil
}

// createContainer pulls image if necessary and runs a container with a database.
func (r *Runner) createContainer(ctx context.Context, state *requestState) error {
	if state.imageFQN == "" || state.imageTag == "" {
		var err error
		state.imageTag, state.imageFQN, err = r.constructImageFQN(state.version)
		if err != nil {
			return fmt.Errorf("failed to construct FQN: %w", err)
		}
	}

	err := r.pull(ctx, state)
	if err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}

	err = r.runContainer(ctx, state)
	if err != nil {
		return fmt.Errorf("container run failed: %w", err)
	}

	return err
}

// pull checks whether the requested image exists. If no, it will be downloaded and renamed to hashed-name.
func (r *Runner) pull(ctx context.Context, state *requestState) (err error) {
	startedAt := time.Now()

	if r.checkIfImageExists(ctx, state) {
		return nil
	}

	out, err := r.engine.pullImage(ctx, state.imageTag)
	if err != nil {
		r.pipelineMetr.PullNewImage(false, state.version, startedAt)
		return errors.Wrap(err, "docker pull failed")
	}

	// We should read the output to be sure that the image has been pulled.
	_, err = io.ReadAll(out)
	if err != nil {
		r.logger.Error().Err(err).Str("image", state.imageTag).Msg("failed to read pull output")
	}

	r.logger.Debug().Str("image", state.imageTag).Msg("base image has been pulled")

	err = r.engine.addImageTag(ctx, state.imageTag, state.imageFQN)
	if err != nil {
		r.pipelineMetr.PullNewImage(false, state.version, startedAt)
		r.logger.Error().Err(err).
			Str("run_id", state.runID).
			Str("source", state.imageTag).
			Str("target", state.imageFQN).
			Msg("failed to rename image")

		return errors.Wrap(err, "failed to tag image")
	}

	r.pipelineMetr.PullNewImage(true, state.version, startedAt)
	r.logger.Debug().
		Str("run_id", state.runID).
		Dur("elapsed_ms", time.Since(startedAt)).
		Str("image", state.imageTag).
		Msg("image has been pulled")

	return nil
}

func (r *Runner) checkIfImageExists(ctx context.Context, state *requestState) bool {
	startedAt := time.Now()

	_, err := r.engine.getImageByID(ctx, state.imageFQN)
	if err == nil {
		r.pipelineMetr.PullExistedImage(true, state.version, startedAt)
		r.logger.Debug().
			Dur("elapsed_ms", time.Since(startedAt)).
			Str("image", state.imageFQN).
			Msg("image has already been pulled")

		return true
	}
	if err != nil && !dockercli.IsErrNotFound(err) {
		r.pipelineMetr.PullExistedImage(false, state.version, startedAt)
		r.logger.Error().Err(err).Str("image", state.imageFQN).Msg("docker inspect failed")
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
		Image:  state.imageFQN,
		Labels: qrunner.CreateContainerLabels(r.name, state.runID, state.version),
	}

	var networkMode string
	if r.cfg.Container.NetworkMode != nil {
		networkMode = *r.cfg.Container.NetworkMode
	}

	// Network is disabled to prevent malicious attacks and to optimize container start up.
	hostConfig := &container.HostConfig{
		NetworkMode: container.NetworkMode(networkMode),
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

	// A custom quotas config is used to avoid DOS.
	if r.cfg.QuotasPath != nil {
		hostConfig.Mounts = append(hostConfig.Mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   *r.cfg.QuotasPath,
			Target:   fmt.Sprintf("/etc/clickhouse-server/users.d/default%s", path.Ext(*r.cfg.QuotasPath)),
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
		Str("image", state.imageFQN).
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

func (r *Runner) execQuery(ctx context.Context, state *requestState) (stdout string, stderr string, err error) {
	invokedAt := time.Now()
	defer func() {
		r.pipelineMetr.ExecCommand(err == nil, state.version, invokedAt)
	}()

	var args []string

	switch state.settings.Type() {
	case database.TypeClickHouse:
		args = []string{
			"clickhouse", "client",
			"-n",
			"-m",
			"--query", state.query,
		}

		settings, ok := state.settings.(*runsettings.ClickHouseSettings)
		if !ok {
			return "", "", errors.Errorf("invalid settings for type %s", state.settings.Type())
		}

		formatArgs := settings.FormatArgs(state.version, r.cfg.DefaultOutputFormat)
		args = append(args, formatArgs...)
	default:
		return "", "", errors.Errorf("unknown settings type %s", state.settings.Type())
	}

	resp, err := r.engine.exec(ctx, state.containerID, args)
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
		stdout, stderr, err = r.execQuery(ctx, state)
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
