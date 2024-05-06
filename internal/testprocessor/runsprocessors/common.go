package runsprocessors

import "time"

type PlaygroundClient interface {
	PostRuns(database string, version string, query string) (time.Duration, error)
}

type Mode string
