package qrunner

// LabelOwnership is set for all containers created by clickhouse-playground.
// Use it to find hanged up containers for garbage collection.
const LabelOwnership = "clickhouse.playground.ownership"

// CreateContainerLabels returns default labels for created containers.
// Use labels to find containers created for ch query running purposes
// and to get some basic information what the image was used to run the container.
func CreateContainerLabels(runnerName string, runID string, version string) map[string]string {
	return map[string]string{
		LabelOwnership:                  "1",
		"clickhouse.playground.run":     runID,
		"clickhouse.playground.version": version,
		"clickhouse.playground.runner":  runnerName,
	}
}
