package runsprocessors

import "clickhouse-playground/internal/testprocessor/runs"

const ParallelMode Mode = "parallel"

type ParallelRunsProcessor struct {
	playgroundClient PlaygroundClient
}

func NewParallelModeProcessor(playgroundClient PlaygroundClient) *ParallelRunsProcessor {
	return &ParallelRunsProcessor{playgroundClient: playgroundClient}
}

func (p *ParallelRunsProcessor) Mode() Mode {
	return ParallelMode
}

func (p *ParallelRunsProcessor) Process(_ *runs.Data) {
	// TODO: implement parallel mode
}
