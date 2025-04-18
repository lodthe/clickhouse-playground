package restapi

import (
	"context"

	"github.com/lodthe/clickhouse-playground/internal/dockertag"
	"github.com/lodthe/clickhouse-playground/internal/queryrun"
)

type TagStorage interface {
	GetAll() []dockertag.Image
	Exists(tag string) bool
}

type QueryRunner interface {
	RunQuery(ctx context.Context, run *queryrun.Run) (string, error)
}
