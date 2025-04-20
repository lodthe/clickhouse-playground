package dockerengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFullImageName(t *testing.T) {
	cases := []struct {
		repository string
		version    string
		want       string
	}{
		{
			repository: "clickhouse/clickhouse-server",
			version:    "latest",
			want:       "clickhouse/clickhouse-server:latest",
		},
		{
			repository: "yandex/clickhouse-server",
			version:    "21.2.4",
			want:       "yandex/clickhouse-server:21.2.4",
		},
		{
			repository: "lodthe/clickhouse-playground",
			version:    "1.4-alpine",
			want:       "lodthe/clickhouse-playground:1.4-alpine",
		},
	}

	for _, tc := range cases {
		got := FullImageName(tc.repository, tc.version)
		assert.Equal(t, tc.want, got, tc.version)
	}
}

func TestPlaygroundImageName(t *testing.T) {
	actual := PlaygroundImageName("clickhouse/clickhouse-playground", "sha256:f321ba3999901412bc2616216a631f")
	expected := "chp-clickhouse/clickhouse-playground:f321ba3999901412bc2616216a631f"

	assert.Equal(t, expected, actual)
}

func TestIsPlaygroundImageName(t *testing.T) {
	chp := PlaygroundImageName("clickhouse/clickhouse-playground", "sha256:f321ba3999901412bc2616216a631f")
	notChp := "clickhouse/clickhouse-playground:21.2.2"

	assert.True(t, IsPlaygroundImageName(chp))
	assert.False(t, IsPlaygroundImageName(notChp))
}
