package coordinator

import (
	"context"
	"math/rand"
	"sync/atomic"
	"time"

	"clickhouse-playground/internal/qrunner"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// Coordinator is a runner that does load balancing among other runners.
// It keeps list of existing runners and dispatches incoming queries to one of them.
//
// At the moment, the coordinator just picks a random runner. There are plans to improve load balancing
// mechanisms in the future: use P2C, send health checks and monitor liveness.
type Coordinator struct {
	ctx    context.Context
	cancel context.CancelFunc

	logger  zerolog.Logger
	started int32

	random *rand.Rand

	runners []*Runner
}

func New(ctx context.Context, logger zerolog.Logger, runners []*Runner) *Coordinator {
	ctx, cancel := context.WithCancel(ctx)

	// It's okay to initialize by setting time, because it's just for load balancing among runners.
	random := rand.New(rand.NewSource(time.Now().UnixNano())) // nolint:gosec

	return &Coordinator{
		ctx:     ctx,
		logger:  logger.With().Str("runner", "coordinator").Logger(),
		random:  random,
		cancel:  cancel,
		runners: runners,
	}
}

func (c *Coordinator) Type() qrunner.Type {
	return qrunner.TypeCoordinator
}

func (c *Coordinator) Name() string {
	return "coordinator"
}

// Start starts underlying runners.
func (c *Coordinator) Start() error {
	if !atomic.CompareAndSwapInt32(&c.started, 0, 1) {
		return errors.New("coordinator has already been started")
	}

	c.logger.Info().Int("count", len(c.runners)).Msg("starting...")

	var totalWeight uint64
	var count uint
	for _, r := range c.runners {
		totalWeight += uint64(r.weight)
		if r.weight == 0 {
			continue
		}

		err := r.underlying.Start()
		if err != nil {
			return errors.Wrapf(err, "%s cannot be started", r.underlying.Name())
		}

		count++
	}

	if totalWeight == 0 {
		return errors.New("total runners weight must be > 0")
	}

	c.logger.Info().Uint("count", count).Msg("underlying runners have been started")

	return nil
}

func (c *Coordinator) Stop() error {
	c.cancel()

	c.logger.Info().Msg("stopping coordinator")

	for _, r := range c.runners {
		if r.weight == 0 {
			continue
		}

		err := r.underlying.Stop()
		if err != nil {
			c.logger.Err(err).Str("underlying", r.underlying.Name()).Msg("runner cannot be stopped")
		}
	}

	c.logger.Info().Msg("coordinator has been stopped")

	return nil
}

// RunQuery proxies queries to one of the underlying runners.
func (c *Coordinator) RunQuery(ctx context.Context, runID string, query string, version string) (string, error) {
	r := c.selectRunner()
	if r == nil {
		return "", errors.New("no alive runners")
	}

	return r.underlying.RunQuery(ctx, runID, query, version)
}

// selectRunner selects a random runner (weight is considered).
// If the weight of r1 is 10 times the weight of r2, r1 is selected 10 times more often.
func (c *Coordinator) selectRunner() *Runner {
	var totalWeight uint64
	for _, r := range c.runners {
		totalWeight += uint64(r.weight)
	}

	if totalWeight == 0 {
		return nil
	}

	rnd := c.random.Uint64() % totalWeight
	for _, r := range c.runners {
		if rnd < uint64(r.weight) {
			return r
		}

		rnd -= uint64(r.weight)
	}

	return nil
}
