package qrunner

type Type string

const (
	TypeCoordinator  Type = "COORDINATOR"
	TypeEC2          Type = "EC2"
	TypeDockerEngine Type = "DOCKER_ENGINE"
)
