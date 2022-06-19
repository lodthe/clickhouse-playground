package coordinator

import (
	"math/rand"
	"sync"
	"time"
)

type balancer struct {
	lock    sync.RWMutex
	runners map[string]*Runner

	random *rand.Rand
}

func newBalancer() *balancer {
	// It's okay to initialize by setting time, because it's just for load balancing among runners.
	random := rand.New(rand.NewSource(time.Now().UnixNano())) // nolint:gosec

	return &balancer{
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

	return true
}

// add excludes a runner from load balancing.
func (b *balancer) remove(r *Runner) {
	b.lock.Lock()
	defer b.lock.Unlock()

	delete(b.runners, r.underlying.Name())
}

type runnerJob = func(r *Runner)

// processJob select an available runner and executes the given job.
// It returns true if a runner has been found.
// There are no available runners when all of them are dead or have concurrency limit exhausted.
func (b *balancer) processJob(job runnerJob) bool {
	runner := b.selectRunner()
	if runner == nil {
		return false
	}

	job(runner)

	return true
}

// selectRunner implements a weighted random choice algorithm and returns a runner.
// If the weight of r1 is 10 times the weight of r2, r1 is selected ~10 times more often.
func (b *balancer) selectRunner() *Runner {
	b.lock.RLock()
	defer b.lock.RUnlock()

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
