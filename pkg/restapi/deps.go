package restapi

import (
	"context"

	"clickhouse-playground/internal/dockertag"
)

type TagStorage interface {
	GetAll() []dockertag.Image
	Exists(tag string) bool
}

type QueryRunner interface {
	RunQuery(ctx context.Context, runID string, query string, version string) (string, error)
}
