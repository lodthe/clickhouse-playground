package serialwd

import (
	"clickhouse-playground/internal/testprocessor/runsprocessors"
	"fmt"

	"clickhouse-playground/internal/testprocessor/runs"
)

const Mode runsprocessors.Mode = "serial-without-delays"

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
	for _, run := range runsData.Runs {
		runResult, err := p.playgroundClient.PostRuns(run.Database, run.Version, run.Query)
		if err != nil {
			fmt.Println("Received error from playground client: %w", err)
		}
		run.TimeElapsed = &runResult
		fmt.Printf("Processed request with result: %s\n", runResult.String())
	}
}
