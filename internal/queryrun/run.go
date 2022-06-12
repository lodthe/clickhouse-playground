package queryrun

import (
	"time"

	"github.com/google/uuid"
)

type Run struct {
	ID string `dynamodbav:"Id"`

	Version string `dynamodbav:"Version"`
	Input   string `dynamodbav:"Input"`
	Output  string `dynamodbav:"Output"`

	CreatedAt     time.Time     `dynamodbav:"CreatedAt"`
	ExecutionTime time.Duration `dynamodbav:"ExecutionTime"`
}

func New(input string, version string) *Run {
	return &Run{
		ID:        uuid.New().String(),
		CreatedAt: time.Now(),
		Input:     input,
		Version:   version,
	}
}
