package dockerengine

// requestState holds information about a processing query execution request.
type requestState struct {
	runID string

	version string
	query   string

	chpImageName string
	containerID  string
}
