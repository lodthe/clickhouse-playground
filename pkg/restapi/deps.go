package restapi

import "clickhouse-playground/internal/dockertag"

type TagStorage interface {
	GetAll() []dockertag.ImageTag
	Exists(tag string) bool
}
