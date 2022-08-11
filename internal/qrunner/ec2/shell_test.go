package ec2

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCmdRunContainer(t *testing.T) {
	cases := []struct {
		repository string
		version    string
		want       string
	}{
		{
			repository: "clickhouse/clickhouse-server",
			version:    "21.2",
			want:       "docker run -d --ulimit nofile=262144:262144 -p 8123 clickhouse/clickhouse-server:21.2",
		},
		{
			repository: "clickhouse/clickhouse-server",
			version:    "latest",
			want:       "docker run -d --ulimit nofile=262144:262144 -p 8123 clickhouse/clickhouse-server:latest",
		},
		{
			repository: "yandex/clickhouse-server",
			version:    "latest-alpine",
			want:       "docker run -d --ulimit nofile=262144:262144 -p 8123 yandex/clickhouse-server:latest-alpine",
		},
	}

	for _, tc := range cases {
		got := cmdRunContainer(tc.repository, tc.version)
		assert.Equal(t, tc.want, got, tc.version)
	}
}

func TestCmdRunQuery(t *testing.T) {
	cases := []struct {
		containerID string
		query       string
		want        string
	}{
		{
			containerID: "2939fbb1c",
			query:       "SELECT * FROM system.clusters;\n\nSELECT 'HELLO'",
			want:        "docker exec 2939fbb1c clickhouse client -n -m --query \"SELECT * FROM system.clusters;\n\nSELECT 'HELLO'\"",
		},
		{
			containerID: "CONTAINER",
			query:       "rm -rf",
			want:        "docker exec CONTAINER clickhouse client -n -m --query \"rm -rf\"",
		},
		{
			containerID: "c3392c129",
			query:       "SELECT 1\" rm -rf \"rm -rf' rm -rf",
			want:        "docker exec c3392c129 clickhouse client -n -m --query \"SELECT 1\\\" rm -rf \\\"rm -rf' rm -rf\"",
		},
	}

	for _, tc := range cases {
		got := cmdRunQuery(tc.containerID, tc.query)
		assert.Equal(t, tc.want, got, tc.query)
	}
}

func TestCmdKillContainer(t *testing.T) {
	assert.Equal(t, cmdKillContainer("id888fa444"), "docker kill id888fa444")
}
