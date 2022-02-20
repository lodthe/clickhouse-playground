package dockertag

import (
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
)

type DockerHubClient interface {
	GetTags(image string) ([]string, error)
}

// Storage caches requests of getting tags of the specified image.
type Storage struct {
	expirationTime time.Duration
	cli            DockerHubClient

	mu    sync.RWMutex
	cache map[string]cacheItem
}

func NewStorage(expirationTime time.Duration, cli DockerHubClient) *Storage {
	return &Storage{
		expirationTime: expirationTime,
		cli:            cli,
		cache:          make(map[string]cacheItem),
	}
}

// GetAll returns all known tags of the given image.
func (f *Storage) GetAll(image string) ([]string, error) {
	return f.get(image)
}

// Exists checks whether the given image has the given tag.
func (f *Storage) Exists(image string, tag string) (bool, error) {
	tags, err := f.get(image)
	if err != nil {
		return false, err
	}

	for _, t := range tags {
		if strings.EqualFold(t, tag) {
			return true, nil
		}
	}

	return false, nil
}

func (f *Storage) get(image string) ([]string, error) {
	f.mu.RLock()
	item, exists := f.cache[image]
	if exists && !item.expired(f.expirationTime) {
		f.mu.RUnlock()
		return item.tags, nil
	}

	f.mu.RUnlock()

	return f.update(image)
}

func (f *Storage) update(image string) ([]string, error) {
	tags, err := f.cli.GetTags(image)
	if err != nil {
		return nil, errors.Wrap(err, "dockerhub request failed")
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.cache[image] = cacheItem{
		tags:    tags,
		savedAt: time.Now(),
	}

	return tags, nil
}
