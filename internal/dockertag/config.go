package dockertag

import "time"

const DefaultExpirationTime = 5 * time.Minute

type Config struct {
	Repositories []string
	OS           string
	Architecture string

	ExpirationTime time.Duration
}
