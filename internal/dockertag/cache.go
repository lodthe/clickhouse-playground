package dockertag

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"clickhouse-playground/pkg/dockerhub"

	zlog "github.com/rs/zerolog/log"
)

type DockerHubClient interface {
	GetTags(image string) ([]dockerhub.ImageTag, error)
}

// Cache is a cache for the list of docker image's tags.
type Cache struct {
	ctx    context.Context
	config Config
	cli    DockerHubClient

	updating int32

	mu         sync.RWMutex
	updatedAt  time.Time
	imageByTag map[string]Image
	images     []Image
}

func NewCache(ctx context.Context, config Config, cli DockerHubClient) *Cache {
	return &Cache{
		ctx:        ctx,
		config:     config,
		cli:        cli,
		imageByTag: make(map[string]Image),
	}
}

// RunBackgroundUpdate runs a background task that keeps data actual.
func (c *Cache) RunBackgroundUpdate() {
	go c.backgroundUpdate()
}

func (c *Cache) backgroundUpdate() {
	update := func() {
		c.mu.RLock()
		defer c.mu.RUnlock()

		c.updateIfExpired()
	}

	zlog.Info().Msg("docker tag cache update background task has been started")

	update()
	t := time.NewTicker(c.config.ExpirationTime)
	defer t.Stop()

	for {
		select {
		case <-c.ctx.Done():
			zlog.Info().Msg("docker tag cache update background task has been finished")
			return

		case <-t.C:
		}

		update()
	}
}

func (c *Cache) normalizeTag(tag string) string {
	return strings.ToLower(tag)
}

// GetAll returns all known tags for the given image.
func (c *Cache) GetAll() []Image {
	c.mu.RLock()
	defer c.mu.RUnlock()

	c.updateIfExpired()

	return c.images
}

// Exists checks whether the image has the given tag.
func (c *Cache) Exists(tag string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, found := c.imageByTag[c.normalizeTag(tag)]

	c.updateIfExpired()

	return found
}

// Find searches an image by its tag.
func (c *Cache) Find(tag string) (img Image, found bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	defer c.updateIfExpired()

	img, found = c.imageByTag[c.normalizeTag(tag)]

	return img, found
}

// updateIfExpired asynchronously updates cache if the cache has expired.
// The function should be called under the acquired mu lock.
func (c *Cache) updateIfExpired() {
	if time.Since(c.updatedAt) < c.config.ExpirationTime {
		return
	}

	// Do nothing if the updating lock has been acquired in another goroutine.
	if !atomic.CompareAndSwapInt32(&c.updating, 0, 1) {
		return
	}

	go c.asyncUpdate()
}

// asyncUpdate fetches actual image list and updates the cache.
// The updating atomic is used to prevent simultaneous updates.
func (c *Cache) asyncUpdate() {
	// Release the acquired lock.
	defer func() {
		atomic.StoreInt32(&c.updating, 0)
	}()

	startedAt := time.Now()

	tags, err := c.cli.GetTags(c.config.Image.Repository)
	if err != nil {
		zlog.Error().Err(err).Str("image", c.config.Image.Repository).Msg("failed to get docker images")
		return
	}

	var flattened []Image
	imgByTag := make(map[string]Image)

	for _, t := range tags {
		for _, i := range t.Images {
			if !strings.EqualFold(i.OS, c.config.Image.OS) || !strings.EqualFold(i.Architecture, c.config.Image.Architecture) {
				continue
			}

			converted := Image{
				Repository:   c.config.Image.Repository,
				Tag:          t.Name,
				OS:           i.OS,
				Architecture: i.Architecture,
				Digest:       i.Digest,
				PushedAt:     i.LastPushed,
			}

			imgByTag[c.normalizeTag(converted.Tag)] = converted
			flattened = append(flattened, converted)
		}
	}

	func() {
		c.mu.Lock()
		defer c.mu.Unlock()

		c.updatedAt = time.Now()
		c.imageByTag = imgByTag
		c.images = flattened
	}()

	zlog.Debug().Dur("elapsed", time.Since(startedAt)).Int("tag_count", len(imgByTag)).Msg("docker image tag cache has been updated")
}
