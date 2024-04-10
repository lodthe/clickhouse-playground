package testprocessor

import (
	"encoding/csv"
	"fmt"
	"os"
	"time"

	zlog "github.com/rs/zerolog/log"
)

type Mode string

const (
	SerialMode              Mode = "serial"
	SerialWithoutDelaysMode Mode = "serial-without-delays"
	ParallelMode            Mode = "parallel"
)

type Config struct {
	Mode         Mode
	RunsDataPath string
	OutputPath   string
	DefaultQuery *string
	Percentiles  []int
}

type PlaygroundClient interface {
	PostRuns(database string, version string, query string) (time.Duration, error)
}

type Processor struct {
	PlaygroundClient PlaygroundClient
	Config           *Config
}

func New(playgroundClient PlaygroundClient, config *Config) *Processor {
	return &Processor{
		PlaygroundClient: playgroundClient,
		Config:           config,
	}
}

func (p *Processor) Process() {
	runs, err := loadRuns(p.Config.RunsDataPath)
	if err != nil {
		zlog.Fatal().Err(err).Msg("runs data cannot be loaded")
	}

	if p.Config.DefaultQuery != nil {
		for _, run := range runs.Runs {
			run.Query = *p.Config.DefaultQuery
		}
	}

	switch p.Config.Mode {
	case SerialMode:
		p.processSerialMode(runs)
	case SerialWithoutDelaysMode:
		p.processSerialWithoutDelaysMode(runs)
	case ParallelMode:
		p.processParallelMode()
	default:
		zlog.Fatal().Msg("unknown mode")
	}

	p.exportTestResult(runs.Runs)

	aggregator := NewAggregator(runs.Runs)
	aggregator.PrintPercentiles(p.Config.Percentiles)
}

func (p *Processor) processSerialMode(runs *RunsInput) {
	for i, run := range runs.Runs {
		runResult, err := p.PlaygroundClient.PostRuns(run.Database, run.Version, run.Query)
		if err != nil {
			zlog.Error().Err(err).Msg("received error from playground client")
		}
		run.TimeElapsed = runResult
		zlog.Info().Msg(fmt.Sprintf("processed request: %s", runResult.String()))

		if i != len(runs.Runs)-1 {
			nextRequestTime := runs.Runs[i+1].Timestamp
			sleepDelta := nextRequestTime.Sub(run.Timestamp)
			time.Sleep(sleepDelta)
		}
	}
}

func (p *Processor) processSerialWithoutDelaysMode(runs *RunsInput) {
	for _, run := range runs.Runs {
		runResult, err := p.PlaygroundClient.PostRuns(run.Database, run.Version, run.Query)
		if err != nil {
			zlog.Error().Err(err).Msg("received error from playground client")
		}
		run.TimeElapsed = runResult
		zlog.Info().Msg(fmt.Sprintf("Processed request: %s", runResult.String()))
	}
}

func (p *Processor) processParallelMode() {
	// TBD
}

func (p *Processor) exportTestResult(runs []*Run) {
	file, err := os.Create(p.Config.OutputPath)
	defer file.Close()

	if err != nil {
		zlog.Error().Err(err).Msg("unable to create output file")
		return
	}

	w := csv.NewWriter(file)
	defer w.Flush()

	if err = w.Write([]string{"database", "version", "query", "elapsed time"}); err != nil {
		zlog.Error().Err(err).Msg("failed to write headers")
		return
	}

	for _, run := range runs {
		if err = w.Write(run.CsvRow()); err != nil {
			zlog.Error().Err(err).Msg("failed to write run's data")
			return
		}
	}
}
