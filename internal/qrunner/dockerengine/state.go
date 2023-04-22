package dockerengine

// requestState holds information about a processing query execution request.
type requestState struct {
	runID string

	version string
	query   string

	// <repository>:<version>
	imageTag string

	// a unique name that refers the image
	imageFQN string

	containerID string
}
