package dockertag

import (
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
	config Config
	cli    DockerHubClient

	updating int32

	mu            sync.RWMutex
	updatedAt     time.Time
	tags          map[string]ImageTag
	tagsFlattened []ImageTag
}

func NewStorage(config Config, cli DockerHubClient) *Cache {
	return &Cache{
		config: config,
		cli:    cli,
		tags:   make(map[string]ImageTag),
	}
}

func (c *Cache) normalizeTag(tag string) string {
	return strings.ToLower(tag)
}

// GetAll returns all known tags for the given image.
func (c *Cache) GetAll() []ImageTag {
	c.mu.RLock()
	defer c.mu.RUnlock()

	c.updateIfExpired()

	return c.tagsFlattened
}

// Exists checks whether the image has the given tag.
func (c *Cache) Exists(tag string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, found := c.tags[c.normalizeTag(tag)]

	c.updateIfExpired()

	return found
}

// Get returns the image tag if it exists. Otherwise, nil is returned.
func (c *Cache) Get(tag string) *ImageTag {
	c.mu.RLock()
	defer c.mu.RUnlock()

	t, found := c.tags[c.normalizeTag(tag)]
	if !found {
		return nil
	}

	c.updateIfExpired()

	return &t
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

	tags, err := c.cli.GetTags(c.config.Image)
	if err != nil {
		zlog.Error().Err(err).Str("image", c.config.Image).Msg("failed to get docker images")
		return
	}

	var tagsFlattened []ImageTag
	tagsByName := make(map[string]ImageTag)

	for _, t := range tags {
		for _, i := range t.Images {
			if !strings.EqualFold(i.OS, c.config.OS) || !strings.EqualFold(i.Architecture, c.config.Architecture) {
				continue
			}

			converted := ImageTag{
				ImageName:    c.config.Image,
				TagName:      t.Name,
				OS:           i.OS,
				Architecture: i.Architecture,
				Digest:       i.Digest,
				PushedAt:     i.LastPushed,
			}

			tagsByName[c.normalizeTag(converted.TagName)] = converted
			tagsFlattened = append(tagsFlattened, converted)
		}
	}

	func() {
		c.mu.Lock()
		defer c.mu.Unlock()

		c.updatedAt = time.Now()
		c.tags = tagsByName
		c.tagsFlattened = tagsFlattened
	}()

	zlog.Debug().Dur("elapsed", time.Since(startedAt)).Int("tag_count", len(tagsByName)).Msg("docker image tag cache has been updated")
}
