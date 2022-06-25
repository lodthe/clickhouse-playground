package dockertag

import "time"

const DefaultExpirationTime = 5 * time.Minute

type Config struct {
	Image ImageConfig

	ExpirationTime time.Duration
}

type ImageConfig struct {
	Repository   string
	OS           string
	Architecture string
}
