package coordinator

import "clickhouse-playground/internal/qrunner"

const DefaultWeight = 100

type Runner struct {
	underlying qrunner.Runner

	// Weight is for load balancing.
	weight uint
}

func NewRunner(underlying qrunner.Runner, weight uint) *Runner {
	return &Runner{
		underlying: underlying,
		weight:     weight,
	}
}
