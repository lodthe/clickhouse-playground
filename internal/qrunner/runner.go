package qrunner

import (
	"clickhouse-playground/internal/clientsettings"
	"context"
)

type Runner interface {
	Type() Type
	Name() string

	Status(ctx context.Context) RunnerStatus

	RunQuery(ctx context.Context, runID string, query string, version string, settings clientsettings.Settings) (string, error)

	// Start initializes background processes (like garbage collection and status exporter).
	// This function is non-blocking.
	Start() error

	// Stop stops background tasks and waits for their finish.
	Stop(shutdownCtx context.Context) error
}
