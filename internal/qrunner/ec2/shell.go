package ec2

import (
	"fmt"
	"strings"

	"clickhouse-playground/internal/qrunner"
)

// cmdRunContainer generates a shell command to run a container with the database.
// Validate repository and version to avoid injections.
func cmdRunContainer(repository, version string) string {
	img := qrunner.FullImageName(repository, version)
	return fmt.Sprintf("docker run -d --ulimit nofile=262144:262144 -p 8123 %s", img)
}

// cmdRunQuery generates a shell command to run a query in the running Docker container.
// TODO: fix shell injection.
func cmdRunQuery(containerID string, query string) string {
	query = strings.ReplaceAll(query, "\"", "\\\"")
	return fmt.Sprintf("docker exec %s clickhouse-client -n -m --query \"%s\"", containerID, query) // nolint
}

// cmdKillContainer generates a shell command to kill the running container.
func cmdKillContainer(containerID string) string {
	return fmt.Sprintf("docker kill %s", containerID)
}
