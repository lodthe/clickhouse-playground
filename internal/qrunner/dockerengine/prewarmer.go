package dockerengine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type containerRunner interface {
	createContainer(ctx context.Context, state *requestState) error
}

type prewarmer struct {
	ctx    context.Context
	cancel context.CancelFunc
	logger zerolog.Logger

	runner containerRunner
	engine *engineProvider

	lock sync.Mutex

	containers          map[string]*containerState
	latestRequestsQueue []requestState
	signals             chan struct{}

	maxWarmContainers uint
}

func newPrewarmer(ctx context.Context, logger zerolog.Logger, runner containerRunner, engine *engineProvider, maxWarmContainers uint) *prewarmer {
	ctx, cancel := context.WithCancel(ctx)

	return &prewarmer{
		ctx:               ctx,
		cancel:            cancel,
		logger:            logger,
		runner:            runner,
		engine:            engine,
		containers:        make(map[string]*containerState),
		signals:           make(chan struct{}, 1),
		maxWarmContainers: maxWarmContainers,
	}
}

func (p *prewarmer) notify() {
	select {
	case p.signals <- struct{}{}:
	default:
	}
}

func (p *prewarmer) Start() error {
	p.logger.Info().Msg("prewarmer has been started")

	for {
		select {
		case <-p.ctx.Done():
			return nil

		case <-p.signals:
		}

		req, count := p.extractNextRequest()
		if count == 0 {
			continue
		}

		err := p.runContainer(&req)
		if err != nil {
			p.logger.Err(err).Str("image", req.imageFQN).Msg("failed to start a prewarmed container")
		}

		// If there are some unprocessed images, they will be processed in
		// the next loop iteration.
		// Images are processed one by one to provide better actualization
		if count > 1 {
			p.notify()
		}
	}
}

func (p *prewarmer) Stop(shutdownCtx context.Context) {
	p.cancel()

	// Release allocated resources: remove all prewarmed containers.
	p.lock.Lock()
	defer p.lock.Unlock()

	p.logger.Info().Int("count", len(p.containers)).Msg("start removing prewarmed containers")

	for _, c := range p.containers {
		err := p.engine.removeContainer(shutdownCtx, c.id, true)
		if err != nil {
			p.logger.Err(err).Str("container_id", c.id).Msg("failed to remove container")
		}
	}

	p.containers = make(map[string]*containerState)

	p.logger.Info().Msg("prewarmer has been stopped")
}

// extractNextRequest extracts the next request and
// the number of requests in the queue, waiting to be processed.
func (p *prewarmer) extractNextRequest() (requestState, int) {
	p.lock.Lock()
	defer p.lock.Unlock()

	cnt := len(p.latestRequestsQueue)
	if cnt == 0 {
		return requestState{}, 0
	}

	req := p.latestRequestsQueue[0]
	p.latestRequestsQueue = p.latestRequestsQueue[1:]

	return req, cnt
}

func (p *prewarmer) runContainer(request *requestState) error {
	state := requestState{
		runID:    "PREWARMING",
		query:    " ",
		version:  request.version,
		imageTag: request.imageTag,
		imageFQN: request.imageFQN,
	}
	err := p.runner.createContainer(p.ctx, &state)
	if err != nil {
		return fmt.Errorf("failed to create a new container: %w", err)
	}

	p.logger.Info().Str("id", state.containerID).Str("image", state.imageFQN).Msg("a new prewarmed container has been created")

	p.lock.Lock()
	defer p.lock.Unlock()

	_, found := p.containers[request.imageFQN]
	if found {
		return errors.New("container for that image already exists")
	}

	container := &containerState{
		id:        state.containerID,
		imageFQN:  state.imageFQN,
		createdAt: time.Now(),
		status:    statusRunning,
	}
	p.containers[request.imageFQN] = container

	// If the number of prewarmed containers exceeds the limit,
	// delete the oldest.
	if len(p.containers) > int(p.maxWarmContainers) {
		oldestImage := container.imageFQN
		oldestDate := container.createdAt
		for fqn, c := range p.containers {
			if c.createdAt.Before(oldestDate) {
				oldestImage = fqn
				oldestDate = c.createdAt
			}
		}

		oldestContainer := p.containers[oldestImage]
		delete(p.containers, oldestImage)

		go func() {
			err := p.engine.removeContainer(p.ctx, oldestContainer.id, true)
			if err != nil {
				p.logger.Err(err).Str("container_id", oldestContainer.id).Msg("failed to remove container")
			}
		}()
	}

	// Pause container after some time to allow its bootstrap.
	go func() {
		release := container.acquireLock()
		defer release()

		if container.status != statusRunning {
			return
		}

		err := p.engine.pauseContainer(p.ctx, container.id)
		if err != nil {
			p.logger.Err(err).Str("container_id", container.id).Msg("failed to pause container")
			return
		}

		container.setStatus(statusPaused)

		p.logger.Info().Str("id", container.id).Str("image", container.imageFQN).Msg("container has been paused")
	}()

	return nil
}

// PushNewRequest should be called when a new request comes.
// It remembers the request and signals the background worker to process this new images.
func (p *prewarmer) PushNewRequest(request requestState) {
	p.lock.Lock()
	defer p.lock.Unlock()

	// If there is such an image in the waiting queue, skip it.
	for _, r := range p.latestRequestsQueue {
		if r.imageFQN == request.imageFQN {
			return
		}
	}

	// Add a new item to the queue and trim the prefix if necessary.
	p.latestRequestsQueue = append(p.latestRequestsQueue, request)
	if len(p.latestRequestsQueue) >= int(p.maxWarmContainers) && len(p.latestRequestsQueue) > 0 {
		p.latestRequestsQueue = p.latestRequestsQueue[1:]
	}

	p.notify()
}

// Fetch returns id of a warm container if it exists.
// It is safe to exec in the returned container immediately
// (containers are unpaused when they are fetched).
func (p *prewarmer) Fetch(imageFQN string) (containerID string, found bool, err error) {
	c := p.extractContainer(imageFQN)
	if c == nil {
		return "", false, nil
	}

	err = p.unpauseIfNecessary(c)
	if err != nil {
		return "", false, err
	}

	return c.id, true, nil
}

func (p *prewarmer) extractContainer(imageFQN string) (container *containerState) {
	p.lock.Lock()
	defer p.lock.Unlock()

	container, found := p.containers[imageFQN]
	if !found {
		return nil
	}

	delete(p.containers, imageFQN)

	return container
}

func (p *prewarmer) unpauseIfNecessary(c *containerState) error {
	release := c.acquireLock()
	defer release()

	if c.status == statusRunning {
		return nil
	}

	err := p.engine.unpauseContainer(p.ctx, c.id)
	if err != nil {
		return fmt.Errorf("unpause failed: %w", err)
	}

	p.logger.Info().Str("id", c.id).Str("image", c.imageFQN).Msg("container has been unpaused")

	c.setStatus(statusFetched)

	return nil
}
