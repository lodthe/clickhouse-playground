package queryrun

import (
	"clickhouse-playground/internal/runsettings"
	"time"

	"github.com/google/uuid"
)

type Run struct {
	ID string `dynamodbav:"Id"`

	Version string `dynamodbav:"Version"`
	Input   string `dynamodbav:"Input"`
	Output  string `dynamodbav:"Output"`

	Database string                  `dynamodbav:"Database"`
	Settings runsettings.RunSettings `dynamodbav:"Settings"`

	CreatedAt     time.Time     `dynamodbav:"CreatedAt"`
	ExecutionTime time.Duration `dynamodbav:"ExecutionTime"`
}

func New(input string, database string, version string, settings runsettings.RunSettings) *Run {
	return &Run{
		ID:        uuid.New().String(),
		CreatedAt: time.Now(),
		Input:     input,
		Database:  database,
		Version:   version,
		Settings:  settings,
	}
}
