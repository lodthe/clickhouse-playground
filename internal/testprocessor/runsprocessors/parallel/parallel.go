package parallel

import (
	"clickhouse-playground/internal/testprocessor/runs"
	"clickhouse-playground/internal/testprocessor/runsprocessors"
)

const Mode runsprocessors.Mode = "parallel"

type Processor struct {
	playgroundClient runsprocessors.PlaygroundClient
}

func New(playgroundClient runsprocessors.PlaygroundClient) *Processor {
	return &Processor{playgroundClient: playgroundClient}
}

func (p *Processor) Mode() runsprocessors.Mode {
	return Mode
}

func (p *Processor) Process(_ *runs.Data) {
	// TODO: implement parallel mode
}
