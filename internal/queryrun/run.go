package queryrun

import "github.com/google/uuid"

type Run struct {
	ID string `dynamodbav:"Id"`

	Input  string `dynamodbav:"Input"`
	Output string `dynamodbav:"Output"`
}

func New(input string) *Run {
	return &Run{
		ID:    uuid.New().String(),
		Input: input,
	}
}
