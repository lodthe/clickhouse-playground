package dockertag

import (
	"context"
	"testing"
	"time"

	"clickhouse-playground/pkg/dockerhub"

	"github.com/pkg/errors"
	zlog "github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
)

type DockerHubClientMock struct {
	images map[string][]dockerhub.ImageTag
}

func (c *DockerHubClientMock) GetTags(repository string) ([]dockerhub.ImageTag, error) {
	images, exists := c.images[repository]
	if !exists {
		return nil, errors.New("not found")
	}

	return images, nil
}

func TestGetImagesFromSeveralRepositories(t *testing.T) {
	config := Config{
		Repositories: []string{
			"a/clickhouse",
			"b/clickhouse",
		},
		OS:             "linux",
		Architecture:   "amd64",
		ExpirationTime: DefaultExpirationTime,
	}
	cli := &DockerHubClientMock{
		images: map[string][]dockerhub.ImageTag{
			"a/clickhouse": {
				{
					Images: []dockerhub.Image{
						{
							Architecture: "linux",
							OS:           "invalid os, should be skipped",
							LastPushed:   time.Now(),
						},
						{
							Architecture: config.Architecture,
							OS:           config.OS,
							LastPushed:   time.Now().Add(-time.Hour),
						},
					},
					Name: "latest",
				},
				{
					Images: []dockerhub.Image{
						{
							Architecture: config.Architecture,
							OS:           config.OS,
							LastPushed:   time.Now().Add(time.Hour),
						},
					},
					Name: "latest-alpine",
				},
			},
			"b/clickhouse": {
				{
					Images: []dockerhub.Image{
						{
							Architecture: config.Architecture,
							OS:           config.OS,
							LastPushed:   time.Now().Add(-time.Hour),
						},
					},
					Name: "latest",
				},
				{
					Images: []dockerhub.Image{
						{
							Architecture: config.Architecture,
							OS:           config.OS,
							LastPushed:   time.Now().Add(2 * time.Hour),
						},
					},
					Name: "21.8",
				},
			},
		},
	}

	cache := NewCache(context.Background(), config, zlog.Logger, cli)

	images, imgByTag, err := cache.getImagesFromSeveralRepositories(config.Repositories)
	assert.NoError(t, err)
	assert.Len(t, images, 3)
	assert.Len(t, imgByTag, 3)

	assert.Equal(t, "latest-alpine", images[0].Tag)
	assert.Equal(t, "latest", images[1].Tag)
	assert.Equal(t, "21.8", images[2].Tag)

	for _, img := range images {
		assert.Equal(t, img, imgByTag[cache.normalizeTag(img.Tag)])
	}
}
