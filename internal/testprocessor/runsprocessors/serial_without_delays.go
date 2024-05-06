package runsprocessors

import (
	"fmt"

	"clickhouse-playground/internal/testprocessor/runs"
)

const SerialWithoutDelaysMode Mode = "serial-without-delays"

type SerialWithoutDelaysRunsProcessor struct {
	playgroundClient PlaygroundClient
}

func NewSerialWithoutDelaysModeProcessor(playgroundClient PlaygroundClient) *SerialWithoutDelaysRunsProcessor {
	return &SerialWithoutDelaysRunsProcessor{playgroundClient: playgroundClient}
}

func (p *SerialWithoutDelaysRunsProcessor) Mode() Mode {
	return SerialWithoutDelaysMode
}

func (p *SerialWithoutDelaysRunsProcessor) Process(runsData *runs.Data) {
	for _, run := range runsData.Runs {
		runResult, err := p.playgroundClient.PostRuns(run.Database, run.Version, run.Query)
		if err != nil {
			fmt.Println("Received error from playground client: %w", err)
		}
		run.TimeElapsed = &runResult
		fmt.Printf("Processed request with result: %s\n", runResult.String())
	}
}
