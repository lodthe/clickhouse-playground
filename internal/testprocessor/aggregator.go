package testprocessor

import (
	"fmt"
	"sort"

	"clickhouse-playground/internal/testprocessor/runs"
)

type Aggregator struct {
	runs []*runs.Run
}

func NewAggregator(runs []*runs.Run) *Aggregator {
	sort.Slice(runs, func(i, j int) bool {
		return *runs[i].TimeElapsed < *runs[j].TimeElapsed
	})
	return &Aggregator{runs: runs}
}

func (a *Aggregator) Percentile(percentile int) (string, error) {
	index := len(a.runs)*percentile/100 - 1

	if index < 0 {
		return "", fmt.Errorf("invalid index %d for percentile %d", index, percentile)
	}

	return a.runs[index].TimeElapsed.String(), nil
}

func (a *Aggregator) PrintPercentiles(percentilesToCalculate []int) {
	for _, perc := range percentilesToCalculate {
		percValue, err := a.Percentile(perc)
		if err != nil {
			fmt.Printf("Failed to calculate percentile %d: %s\n", perc, err)
		}

		fmt.Printf("Percentile %d: %s\n", perc, percValue)
	}
}
