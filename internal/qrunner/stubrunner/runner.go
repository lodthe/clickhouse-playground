package stubrunner

import (
	"context"

	"clickhouse-playground/internal/qrunner"

	"github.com/pkg/errors"
)

type Run = func(ctx context.Context, runID string, query string, version string) (string, error)

var StubRun = func(ctx context.Context, runID string, query string, version string) (string, error) {
	return "", errors.New("stub cannot run queries")
}

// Runner is a stub runner for tests.
type Runner struct {
	ctx  context.Context
	name string

	run Run
}

func New(ctx context.Context, name string, run Run) *Runner {
	return &Runner{
		ctx:  ctx,
		name: name,
		run:  run,
	}
}

func (r *Runner) Type() qrunner.Type {
	return qrunner.TypeStub
}

func (r *Runner) Name() string {
	return r.name
}

func (r *Runner) Status(_ context.Context) qrunner.RunnerStatus {
	return qrunner.RunnerStatus{
		Alive: true,
	}
}

func (r *Runner) Start() error {
	return nil
}

func (r *Runner) Stop(_ context.Context) error {
	return nil
}

func (r *Runner) RunQuery(ctx context.Context, runID string, query string, version string) (string, error) {
	return r.run(ctx, runID, query, version)
}
