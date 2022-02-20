package dockertag

import (
	"time"
)

type cacheItem struct {
	tags    []string
	savedAt time.Time
}

func (c *cacheItem) expired(expirationTime time.Duration) bool {
	return time.Since(c.savedAt) > expirationTime
}
