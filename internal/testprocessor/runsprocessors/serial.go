package runsprocessors

import (
	"fmt"
	"time"

	"clickhouse-playground/internal/testprocessor/runs"
)

const SerialMode Mode = "serial"

type SerialRunsProcessor struct {
	playgroundClient PlaygroundClient
}

func NewSerialModeProcessor(playgroundClient PlaygroundClient) *SerialRunsProcessor {
	return &SerialRunsProcessor{playgroundClient: playgroundClient}
}

func (p *SerialRunsProcessor) Mode() Mode {
	return SerialMode
}

func (p *SerialRunsProcessor) Process(runs *runs.Data) {
	for i, run := range runs.Runs {
		runResult, err := p.playgroundClient.PostRuns(run.Database, run.Version, run.Query)
		if err != nil {
			fmt.Printf("Received error from playground client: %s\n", err)
		}
		run.TimeElapsed = &runResult
		fmt.Printf("Processed request with result: %s\n", runResult.String())

		if i != len(runs.Runs)-1 {
			nextRequestTime := runs.Runs[i+1].Timestamp
			sleepDelta := nextRequestTime.Sub(run.Timestamp)
			time.Sleep(sleepDelta)
		}
	}
}
