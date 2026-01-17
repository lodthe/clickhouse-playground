package runsettings

import (
	"github.com/lodthe/clickhouse-playground/internal/dbsettings"
	"github.com/lodthe/clickhouse-playground/pkg/chspec"
)

// ClickHouseSettings contains settings for clickhouse client
type ClickHouseSettings struct {
	OutputFormat string `dynamodbav:"OutputFormat"`
}

func (cs *ClickHouseSettings) Type() dbsettings.Type {
	return dbsettings.TypeClickHouse
}

// FormatArgs gets args for custom output formatting
//
// Returns empty args if database version doesn't support --format flag
func (cs *ClickHouseSettings) FormatArgs(version string, defaultOutputFormat string) []string {
	var result []string

	// Check if database version supports --format flag
	if chspec.IsAtLeastMajor(version, "21") {
		outputFormat := defaultOutputFormat
		if cs.OutputFormat != "" {
			outputFormat = cs.OutputFormat
		}

		result = append(result,
			"--output_format_pretty_color", "0",
			"--format", outputFormat)
	}

	return result
}
