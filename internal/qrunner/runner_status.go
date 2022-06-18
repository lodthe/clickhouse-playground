package qrunner

type RunnerStatus struct {
	// If a runner daemon does not respond, the runner is not alive.
	Alive            bool
	LivenessProbeErr error
}
