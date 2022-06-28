package dockertag

import "time"

type Image struct {
	Repository string
	Tag        string

	OS           string
	Architecture string
	Digest       string

	PushedAt time.Time
}
