package qrunner

import (
	"context"
)

type Runner interface {
	Type() Type
	Name() string
	RunQuery(ctx context.Context, runID string, query string, version string) (string, error)
	StartGarbageCollector()
}
