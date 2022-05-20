package qrunner

import (
	"context"
)

type Runner interface {
	RunQuery(ctx context.Context, runID string, query string, version string) (string, error)
	StartGarbageCollector()
}
