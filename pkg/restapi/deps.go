package restapi

import (
	"clickhouse-playground/internal/queryrun"
	"context"

	"clickhouse-playground/internal/dockertag"
)

type TagStorage interface {
	GetAll() []dockertag.Image
	Exists(tag string) bool
}

type QueryRunner interface {
	RunQuery(ctx context.Context, run *queryrun.Run) (string, error)
}
