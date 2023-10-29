package clientsettings

// Settings holds settings for clickhouse client tool.
type Settings struct {
	// Whether to use ANSI escape sequences to paint colors in Pretty formats.
	// Enabled by default.
	OutputFormatPrettyColor string

	// Allows changing a charset which is used for printing grids borders.
	// Available charsets are UTF-8, ASCII.
	OutputFormatPrettyGridCharset string

	// Default output format.
	// Available options: https://clickhouse.com/docs/en/interfaces/formats.
	Format string
}
