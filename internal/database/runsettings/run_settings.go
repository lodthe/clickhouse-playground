package runsettings

import "clickhouse-playground/internal/database"

// RunSettings interface define custom settings for different databases
type RunSettings interface {
	Type() database.Type
}
