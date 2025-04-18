package runsettings

import "clickhouse-playground/internal/dbsettings"

// RunSettings interface define custom settings for different databases
type RunSettings interface {
	Type() dbsettings.Type
}
