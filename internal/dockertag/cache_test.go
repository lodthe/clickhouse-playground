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

	assert.Equal(t, "latest", images[0].Tag)
	assert.Equal(t, "latest-alpine", images[1].Tag)
	assert.Equal(t, "21.8", images[2].Tag)

	for _, img := range images {
		assert.Equal(t, img, imgByTag[cache.normalizeTag(img.Tag)])
	}
}

func TestSortImages(t *testing.T) {
	tests := []struct {
		name   string
		input  []string
		wanted []string
	}{
		{
			name:   "a lot of different images",
			input:  []string{"1.2.3", "12.3.4", "1", "0.2", "head", "5.2.1", "5", "head-alpine"},
			wanted: []string{"head", "head-alpine", "12.3.4", "5.2.1", "5", "1.2.3", "1", "0.2"},
		},
		{
			name:   "must be dropped",
			input:  []string{"1.2.3", "12334", "head"},
			wanted: []string{"head", "1.2.3"},
		},
		{
			name:   "no priority images",
			input:  []string{"1.2.3", "12.3.4", "1", "0.2", "5.2.1", "5"},
			wanted: []string{"12.3.4", "5.2.1", "5", "1.2.3", "1", "0.2"},
		},
		{
			name:   "with semver incompatible",
			input:  []string{"1.2.3", "12.3.4", "1", "lololo.incompatible", "0.2", "head", "5.2.1", "head-alpine", "1.alpha"},
			wanted: []string{"head", "head-alpine", "1.alpha", "lololo.incompatible", "12.3.4", "5.2.1", "1.2.3", "1", "0.2"},
		},
	}

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
		images: make(map[string][]dockerhub.ImageTag),
	}

	cache := NewCache(context.Background(), config, zlog.Logger, cli)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			imgByTag := make(map[string]Image, len(test.input))
			for _, tag := range test.input {
				imgByTag[tag] = Image{Tag: tag}
			}

			sorted := cache.sortImages(imgByTag)

			assert.Len(t, sorted, len(test.wanted))

			for i, expectedTag := range test.wanted {
				assert.Equal(t, expectedTag, sorted[i].Tag, "invalid tag #%d", i)
			}
		})
	}
}
