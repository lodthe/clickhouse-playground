package qrunner

import (
	"clickhouse-playground/internal/queryrun"

	"context"
)

type Runner interface {
	Type() Type
	Name() string

	Status(ctx context.Context) RunnerStatus

	RunQuery(ctx context.Context, run *queryrun.Run) (string, error)

	// Start initializes background processes (like garbage collection and status exporter).
	// This function is non-blocking.
	Start() error

	// Stop stops background tasks and waits for their finish.
	Stop(shutdownCtx context.Context) error
}
