package testprocessor

import (
	"fmt"
	"sort"

	"clickhouse-playground/internal/testprocessor/runs"
)

type Aggregator struct {
	runsData []*runs.Run
}

func NewAggregator(runsData []*runs.Run) *Aggregator {
	sort.Slice(runsData, func(i, j int) bool {
		return *runsData[i].TimeElapsed < *runsData[j].TimeElapsed
	})
	return &Aggregator{runsData: runsData}
}

func (a *Aggregator) Percentile(percentile int) (string, error) {
	index := len(a.runsData)*percentile/100 - 1

	if index < 0 {
		return "", fmt.Errorf("invalid index %d for percentile %d", index, percentile)
	}

	return a.runsData[index].TimeElapsed.String(), nil
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
