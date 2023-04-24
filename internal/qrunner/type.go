package qrunner

type Type string

const (
	TypeCoordinator  Type = "COORDINATOR"
	TypeStub         Type = "STUB"
	TypeDockerEngine Type = "DOCKER_ENGINE"
)
