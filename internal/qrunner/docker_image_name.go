package qrunner

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
