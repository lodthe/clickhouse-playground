package dockertag

import (
	"context"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"clickhouse-playground/pkg/dockerhub"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

type DockerHubClient interface {
	GetTags(repository string) ([]dockerhub.ImageTag, error)
}

// Cache is a cache for the list of docker image's tags.
type Cache struct {
	ctx    context.Context
	config Config
	logger zerolog.Logger
	cli    DockerHubClient

	updating int32

	mu         sync.RWMutex
	updatedAt  time.Time
	imageByTag map[string]Image
	images     []Image
}

func NewCache(ctx context.Context, config Config, logger zerolog.Logger, cli DockerHubClient) *Cache {
	return &Cache{
		ctx:        ctx,
		config:     config,
		logger:     logger,
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

	c.logger.Info().Msg("docker tag cache update background task has been started")

	update()
	t := time.NewTicker(c.config.ExpirationTime)
	defer t.Stop()

	for {
		select {
		case <-c.ctx.Done():
			c.logger.Info().Msg("docker tag cache update background task has been finished")
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
//
// The updating atomic is used to prevent simultaneous updates.
func (c *Cache) asyncUpdate() {
	// Release the acquired lock.
	defer func() {
		atomic.StoreInt32(&c.updating, 0)
	}()

	startedAt := time.Now()

	images, imgByTag, err := c.getImagesFromSeveralRepositories(c.config.Repositories)
	if err != nil {
		return
	}

	func() {
		c.mu.Lock()
		defer c.mu.Unlock()

		c.updatedAt = time.Now()
		c.images = images
		c.imageByTag = imgByTag
	}()

	c.logger.Debug().Dur("elapsed", time.Since(startedAt)).Int("tag_count", len(imgByTag)).Msg("docker image cache has been updated")
}

// getImagesFromSeveralRepositories fetches images from the given list of repositories.
//
// It spawns a goroutine for each repository that collects images from it.
// Then it merges all the lists of images. If there are several occurrences of an image tag in two repositories,
// the data is taken from the first repository.
//
// It returns a list of images and a map that links an image to its tag.
func (c *Cache) getImagesFromSeveralRepositories(repositories []string) ([]Image, map[string]Image, error) {
	g, _ := errgroup.WithContext(c.ctx)
	imagesByRepo := make([][]Image, len(repositories))
	for i := range repositories {
		i := i

		g.Go(func() error {
			images, err := c.getImages(repositories[i])
			if err != nil {
				return err
			}

			imagesByRepo[i] = images

			return nil
		})
	}

	err := g.Wait()
	if err != nil {
		c.logger.Err(err).Msg("failed to update docker image cache")
		return nil, nil, err
	}

	var merged []Image
	imgByTag := make(map[string]Image)
	for _, images := range imagesByRepo {
		for _, img := range images {
			tag := c.normalizeTag(img.Tag)

			// If a tag is presented in several repositories, we save image from the first repo.
			_, exists := imgByTag[tag]
			if exists {
				continue
			}

			imgByTag[tag] = img
			merged = append(merged, img)
		}
	}
	
	sort.Slice(merged, func(i, j int) bool {
    		return merged[i].Tag > merged[j].Tag
	})

	return merged, imgByTag, nil
}

// getImages returns a list of images from the given dockerhub repository.
// It fetches all images and filters them by the supported OS and architecture.
func (c *Cache) getImages(repository string) ([]Image, error) {
	tags, err := c.cli.GetTags(repository)
	if err != nil {
		c.logger.Error().Err(err).Str("repository", repository).Msg("failed to get dockerhub tags")
		return nil, errors.Wrap(err, "failed to get tags from dockerhub")
	}

	c.logger.Debug().Str("repository", repository).Msg("start fetching images")

	var images []Image

	for _, t := range tags {
		for _, i := range t.Images {
			if !strings.EqualFold(i.OS, c.config.OS) || !strings.EqualFold(i.Architecture, c.config.Architecture) {
				continue
			}

			converted := Image{
				Repository:   repository,
				Tag:          t.Name,
				OS:           i.OS,
				Architecture: i.Architecture,
				Digest:       i.Digest,
				PushedAt:     i.LastPushed,
			}

			images = append(images, converted)
		}
	}

	c.logger.Debug().Str("repository", repository).Int("count", len(images)).Msg("images have been fetched")

	sort.Slice(images, func(i, j int) bool {
		return images[i].PushedAt.After(images[j].PushedAt)
	})

	return images, nil
}
