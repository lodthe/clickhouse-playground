package dockertag

import "time"

type ImageTag struct {
	ImageName string
	TagName   string

	OS           string
	Architecture string
	Digest       string

	PushedAt time.Time
}
