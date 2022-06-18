package coordinator

import "time"

type Config struct {
	HealthChecksEnabled bool

	// Delay between two health checks to a runner.
	HealthCheckRetryDelay time.Duration
}

const DefaultHealthCheckRetryDelay = 10 * time.Second
