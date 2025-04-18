package coordinator

import (
	"math/rand"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

type balancer struct {
	logger zerolog.Logger

	lock    sync.Mutex
	runners map[string]*Runner

	random *rand.Rand
}

func newBalancer(logger zerolog.Logger) *balancer {
	// It's okay to initialize by setting time, because it's just for load balancing among runners.
	random := rand.New(rand.NewSource(time.Now().UnixNano())) // nolint:gosec

	return &balancer{
		logger:  logger,
		runners: make(map[string]*Runner),
		random:  random,
	}
}

// add includes a new runner in load balancing if it hasn't been added yet.
// It returns whether the runner hasn't already been added.
func (b *balancer) add(r *Runner) bool {
	b.lock.Lock()
	defer b.lock.Unlock()

	_, found := b.runners[r.underlying.Name()]
	if found {
		return false
	}

	b.runners[r.underlying.Name()] = r

	b.logger.Info().Str("name", r.underlying.Name()).Msg("runner has been included in load balancing")

	return true
}

// remove excludes a runner from load balancing.
func (b *balancer) remove(r *Runner) {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.removeUnderLock(r)
}

func (b *balancer) removeUnderLock(r *Runner) {
	_, found := b.runners[r.underlying.Name()]
	if !found {
		return
	}

	delete(b.runners, r.underlying.Name())

	b.logger.Info().Str("name", r.underlying.Name()).Msg("runner has been excluded from load balancing")
}

type runnerJob = func(r *Runner)

// processJob select an available runner and executes the given job.
// It returns true if a runner has been found.
// There are no available runners when all of them are dead or have concurrency limit exhausted.
func (b *balancer) processJob(job runnerJob) bool {
	var runner *Runner
	var excluded bool
	func() {
		b.lock.Lock()
		defer b.lock.Unlock()

		runner = b.selectRunner()
		if runner == nil {
			return
		}

		// Check if concurrency limit has not been exhausted.
		concurrency := runner.addConcurrency(1)
		if runner.maxConcurrency != nil && concurrency >= *runner.maxConcurrency {
			b.removeUnderLock(runner)
			excluded = true
		}
	}()

	if runner == nil {
		return false
	}

	defer runner.addConcurrency(-1)
	if excluded {
		defer b.add(runner)
	}

	job(runner)

	return true
}

// selectRunner implements a weighted random choice algorithm and returns a runner.
// If the weight of r1 is 10 times the weight of r2, r1 is selected ~10 times more often.
//
// selectRunner must be called under the taken lock.
func (b *balancer) selectRunner() *Runner {
	var totalWeight uint64
	for _, r := range b.runners {
		totalWeight += uint64(r.weight)
	}

	if totalWeight == 0 {
		return nil
	}

	rnd := b.random.Uint64() % totalWeight
	for _, r := range b.runners {
		if rnd < uint64(r.weight) {
			return r
		}

		rnd -= uint64(r.weight)
	}

	return nil
}
