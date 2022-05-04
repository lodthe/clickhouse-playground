package dockertag

import "time"

type Config struct {
	Image        string
	OS           string
	Architecture string

	ExpirationTime time.Duration
}
