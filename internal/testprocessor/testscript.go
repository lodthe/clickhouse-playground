package testprocessor

import (
	"fmt"
	"os"
	"time"

	"clickhouse-playground/internal/testprocessor/runs"
	"clickhouse-playground/internal/testprocessor/runsprocessors"

	"gopkg.in/yaml.v3"

	"github.com/pkg/errors"
)

var ErrUnknownMode = errors.New("unknown mode")

type Config struct {
	Mode         runsprocessors.Mode
	RunsDataPath string
	OutputPath   string
	DefaultQuery *string
	Percentiles  []int
}

type Processor struct {
	Config *Config
}

type PlaygroundClient interface {
	PostRuns(database string, version string, query string) (time.Duration, error)
}

type RunsProcessor interface {
	Process(runsData *runs.Data)
}

func New(config *Config) *Processor {
	return &Processor{
		Config: config,
	}
}

func (p *Processor) Process(playgroundClient PlaygroundClient) error {
	inputRuns, err := runs.LoadRuns(p.Config.RunsDataPath)
	if err != nil {
		return fmt.Errorf("runs data cannot be loaded: %w", err)
	}

	if p.Config.DefaultQuery != nil {
		for _, run := range inputRuns.Runs {
			run.Query = *p.Config.DefaultQuery
		}
	}

	var runsProcessor RunsProcessor

	switch p.Config.Mode {
	case runsprocessors.SerialMode:
		runsProcessor = runsprocessors.NewSerialModeProcessor(playgroundClient)
	case runsprocessors.SerialWithoutDelaysMode:
		runsProcessor = runsprocessors.NewSerialWithoutDelaysModeProcessor(playgroundClient)
	case runsprocessors.ParallelMode:
		runsProcessor = runsprocessors.NewParallelModeProcessor(playgroundClient)
	default:
		return ErrUnknownMode
	}

	runsProcessor.Process(inputRuns)

	err = p.exportTestResult(inputRuns)
	if err != nil {
		return fmt.Errorf("failed to export test results: %w", err)
	}

	aggregator := NewAggregator(inputRuns.Runs)
	aggregator.PrintPercentiles(p.Config.Percentiles)

	return nil
}

func (p *Processor) exportTestResult(runsData *runs.Data) error {
	outputFile, err := os.Create(p.Config.OutputPath)
	if err != nil {
		return fmt.Errorf("unable to create output file: %w", err)
	}

	defer outputFile.Close()

	yamlFile, err := yaml.Marshal(runsData)
	if err != nil {
		return fmt.Errorf("failed to marshal runs to yaml: %w", err)
	}

	_, err = outputFile.WriteString(string(yamlFile))
	if err != nil {
		return fmt.Errorf("failed to write data to output file: %w", err)
	}

	return nil
}
