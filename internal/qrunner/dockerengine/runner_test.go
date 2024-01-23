package dockerengine

import (
	"context"
	"testing"
	"time"

	"clickhouse-playground/internal/dockertag"
	"clickhouse-playground/internal/queryrun"
	"clickhouse-playground/internal/runsettings"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
)

type tagStorageMock struct {
	images map[string]dockertag.Image
}

func (t tagStorageMock) Find(version string) (dockertag.Image, bool) {
	if _, ok := t.images[version]; !ok {
		return dockertag.Image{}, false
	}

	return t.images[version], true
}

func TestCustomSettings(t *testing.T) {
	ctx := context.Background()
	logger := zlog.Logger.Level(zerolog.ErrorLevel)

	tagStorage := tagStorageMock{
		images: map[string]dockertag.Image{
			"21": {
				Repository:   "yandex/clickhouse-server",
				Tag:          "21",
				OS:           "linux",
				Architecture: "amd64",
				Digest:       "sha256:edfee043e4f909dd471c6e282ce3cfd0ce90a4cad3fc234cb27633debe26ea05",
				PushedAt:     time.Now(),
			},
			"20": {
				Repository:   "yandex/clickhouse-server",
				Tag:          "21",
				OS:           "linux",
				Architecture: "amd64",
				Digest:       "sha256:c03c136ca0e87f9b821718f05dc45dea413946ff650ad15980ea89d1c34c87d3",
				PushedAt:     time.Now(),
			},
		},
	}

	cases := []struct {
		database       string
		version        string
		query          string
		expectedOutput string
		runSettings    runsettings.RunSettings
	}{
		{
			database:       "clickhouse",
			version:        "21",
			query:          "SELECT 1",
			expectedOutput: "1\n",
			runSettings:    &runsettings.ClickHouseSettings{},
		},
		{
			database:       "clickhouse",
			version:        "21",
			query:          "SELECT 1",
			expectedOutput: "+-1-+\n| 1 |\n+---+\n",
			runSettings:    &runsettings.ClickHouseSettings{OutputFormat: "PrettyCompactMonoBlock"},
		},
		{
			database:       "clickhouse",
			version:        "20",
			query:          "SELECT 1",
			expectedOutput: "1\n",
			runSettings:    &runsettings.ClickHouseSettings{OutputFormat: "PrettyCompactMonoBlock"},
		},
	}

	rcfg := DefaultConfig
	runner, _ := New(ctx, logger, "Test", rcfg, tagStorage)

	for _, tc := range cases {
		output, err := runner.RunQuery(ctx, &queryrun.Run{Input: tc.query, Version: tc.version, Database: tc.database, Settings: tc.runSettings})
		if err != nil {
			t.Log(err.Error())
		}
		assert.Equal(t, tc.expectedOutput, output)
	}
}
