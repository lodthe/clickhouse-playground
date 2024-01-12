package runsettings

import (
	"clickhouse-playground/pkg/chsemver"
)

// ClickHouseSettings contains settings for clickhouse client
type ClickHouseSettings struct {
	OutputFormat string
}

func (s *ClickHouseSettings) Type() Type {
	return TypeClickHouse
}

// GetFormatArgs gets args for custom output formatting
//
// Returns empty args if database version doesn't support --format flag
func (s *ClickHouseSettings) GetFormatArgs(version string, defaultOutputFormat string) []string {
	var result []string

	if chsemver.IsAtLeastMajor(version, "21") {
		outputFormat := defaultOutputFormat
		if s.OutputFormat != "" {
			outputFormat = s.OutputFormat
		}

		result = append(result,
			"--output_format_pretty_color", "0",
			"--output_format_pretty_grid_charset", "ASCII",
			"--format", outputFormat)
	}

	return result
}
