package coordinator

import (
	"context"
	"math"
	"testing"

	zlog "github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
)

func TestCoordinator_selectRunner_EqualWeights(t *testing.T) {
	const runnerCount = 5
	const samples = 10000
	const maxDeviation = 0.1
	const expected = samples / runnerCount
	// Each runner should be selected samples / runnerCount times roughly.

	var runners []*Runner
	for i := 0; i < runnerCount; i++ {
		runners = append(runners, NewRunner(nil, 100))
	}

	c := New(context.Background(), zlog.Logger, runners)

	timesSelected := make(map[*Runner]uint, len(runners))
	for i := 0; i < samples; i++ {
		r := c.selectRunner()
		timesSelected[r] = timesSelected[r] + 1
	}

	for _, r := range runners {
		count := timesSelected[r]

		deviation := math.Abs(float64(count)/expected - 1)
		assert.LessOrEqual(t, deviation, maxDeviation)
	}
}

func TestCoordinator_selectRunner_DifferentWeights(t *testing.T) {
	const runnerCount = 5
	const samples = 10000
	const maxDeviation = 0.1

	var runners []*Runner
	var totalWeight float64

	// The weight of the i-th runner is (i + 1) * 100.
	for i := 0; i < runnerCount; i++ {
		r := NewRunner(nil, 100*uint(i+1))
		runners = append(runners, r)
		totalWeight += float64(r.weight)
	}

	c := New(context.Background(), zlog.Logger, runners)

	timesSelected := make(map[*Runner]uint, len(runners))
	for i := 0; i < samples; i++ {
		r := c.selectRunner()
		timesSelected[r] = timesSelected[r] + 1
	}

	for i, r := range runners {
		count := timesSelected[r]
		expected := samples * (float64(i+1) * 100 / totalWeight)

		deviation := math.Abs(float64(count)/expected - 1)
		assert.LessOrEqual(t, deviation, maxDeviation)
	}
}
