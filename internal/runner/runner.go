package runner

import (
	"context"
)

type Runner interface {
	RunQuery(ctx context.Context, query string, version string) (string, error)
}
