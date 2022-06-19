package coordinator

import (
	"context"
	"fmt"
	"math"
	"testing"

	"clickhouse-playground/internal/qrunner/stubrunner"

	zlog "github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
)

func TestBalancer_selectRunner_EqualWeights(t *testing.T) {
	const runnerCount = 5
	const samples = 10000
	const maxDeviation = 0.1
	const expected = samples / runnerCount
	// Each runner should be selected samples / runnerCount times roughly.

	ctx := context.Background()
	b := newBalancer(zlog.Logger)

	var runners []*Runner
	for i := 0; i < runnerCount; i++ {
		r := NewRunner(stubrunner.New(ctx, fmt.Sprintf("r%d", i), stubrunner.StubRun), 100)

		assert.True(t, b.add(r))
		runners = append(runners, r)
	}

	timesSelected := make(map[*Runner]uint, len(runners))
	for i := 0; i < samples; i++ {
		r := b.selectRunner()
		timesSelected[r] = timesSelected[r] + 1
	}

	for _, r := range runners {
		count := timesSelected[r]

		deviation := math.Abs(float64(count)/expected - 1)
		assert.LessOrEqual(t, deviation, maxDeviation)
	}
}

func TestBalancer_selectRunner_DifferentWeights(t *testing.T) {
	const runnerCount = 5
	const samples = 20000
	const maxDeviation = 0.2

	var runners []*Runner
	var totalWeight float64

	ctx := context.Background()
	b := newBalancer(zlog.Logger)

	// The weight of the i-th runner is (i + 1) * 100.
	for i := 0; i < runnerCount; i++ {
		r := NewRunner(stubrunner.New(ctx, fmt.Sprintf("r%d", i), stubrunner.StubRun), 100*uint(i+1))
		runners = append(runners, r)
		assert.True(t, b.add(r))
		totalWeight += float64(r.weight)
	}

	timesSelected := make(map[*Runner]uint, len(runners))
	for i := 0; i < samples; i++ {
		r := b.selectRunner()
		timesSelected[r] = timesSelected[r] + 1
	}

	for i, r := range runners {
		count := timesSelected[r]
		expected := samples * (float64(i+1) * 100 / totalWeight)

		deviation := math.Abs(float64(count)/expected - 1)
		assert.LessOrEqual(t, deviation, maxDeviation)
	}
}
