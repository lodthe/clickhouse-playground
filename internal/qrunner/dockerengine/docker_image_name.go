package dockerengine

import (
	"fmt"
	"strings"
)

func FullImageName(repository, version string) string {
	return fmt.Sprintf("%s:%s", repository, version)
}

func PlaygroundImageName(repository, digest string) string {
	return fmt.Sprintf("chp-%s:%s", repository, strings.TrimPrefix(digest, "sha256:"))
}

func IsPlaygroundImageName(name string) bool {
	return strings.HasPrefix(name, "chp-")
}
