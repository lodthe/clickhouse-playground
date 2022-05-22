package queryrun

import "github.com/google/uuid"

type Run struct {
	ID string `dynamodbav:"Id"`

	Version string `dynamodbav:"Version"`

	Input  string `dynamodbav:"Input"`
	Output string `dynamodbav:"Output"`
}

func New(input string, version string) *Run {
	return &Run{
		ID:      uuid.New().String(),
		Input:   input,
		Version: version,
	}
}
