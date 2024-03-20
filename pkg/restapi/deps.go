package restapi

import (
	"context"

	"clickhouse-playground/internal/dockertag"
	"clickhouse-playground/internal/queryrun"
)

type TagStorage interface {
	GetAll() []dockertag.Image
	Exists(tag string) bool
}

type QueryRunner interface {
	RunQuery(ctx context.Context, run *queryrun.Run) (string, error)
}
