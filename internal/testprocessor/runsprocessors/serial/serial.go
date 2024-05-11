package serial

import (
	"clickhouse-playground/internal/testprocessor/runsprocessors"
	"fmt"
	"time"

	"clickhouse-playground/internal/testprocessor/runs"
)

const Mode runsprocessors.Mode = "serial"

type Processor struct {
	playgroundClient runsprocessors.PlaygroundClient
}

func New(playgroundClient runsprocessors.PlaygroundClient) *Processor {
	return &Processor{playgroundClient: playgroundClient}
}

func (p *Processor) Mode() runsprocessors.Mode {
	return Mode
}

func (p *Processor) Process(runsData *runs.Data) {
	for i, run := range runsData.Runs {
		runResult, err := p.playgroundClient.PostRuns(run.Database, run.Version, run.Query)
		if err != nil {
			fmt.Printf("Received error from playground client: %s\n", err)
		}
		run.TimeElapsed = &runResult
		fmt.Printf("Processed request with result: %s\n", runResult.String())

		if i != len(runsData.Runs)-1 {
			nextRequestTime := runsData.Runs[i+1].Timestamp
			sleepDelta := nextRequestTime.Sub(run.Timestamp)
			time.Sleep(sleepDelta)
		}
	}
}
