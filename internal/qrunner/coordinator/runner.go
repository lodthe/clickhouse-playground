package coordinator

import (
	"sync/atomic"

	"clickhouse-playground/internal/qrunner"
)

const DefaultWeight = 100

type Runner struct {
	underlying qrunner.Runner

	// Liveness is controlled by ping probes.
	alive uint32

	// Weight is for load balancing.
	weight uint
}

func NewRunner(underlying qrunner.Runner, weight uint) *Runner {
	return &Runner{
		underlying: underlying,
		weight:     weight,
		alive:      0,
	}
}

func (r *Runner) IsAlive() bool {
	return atomic.LoadUint32(&r.alive) == 1
}

func (r *Runner) setAlive(alive bool) {
	var converted uint32
	if alive {
		converted = 1
	}

	atomic.StoreUint32(&r.alive, converted)
}
