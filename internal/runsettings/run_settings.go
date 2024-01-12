package runsettings

// RunSettings interface define custom settings for different databases
type RunSettings interface {
	Type() Type
}
