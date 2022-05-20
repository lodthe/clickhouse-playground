package localdocker

import "time"

type Config struct {
	ExecRetryDelay time.Duration
	MaxExecRetries int

	// Path to the xml or yaml config which will be added to the ../config.d/ directory.
	CustomConfigPath *string

	GC *GCConfig
}

type GCConfig struct {
	// How often GC will be triggered.
	TriggerFrequency time.Duration

	ImageBufferSize int
	ContainerTTL    *time.Duration
}

var defeaultContainerTTL = 60 * time.Second
var DefaultLocalDockerConfig = Config{
	ExecRetryDelay: 200 * time.Millisecond,
	MaxExecRetries: 20,

	CustomConfigPath: nil,

	GC: &GCConfig{
		TriggerFrequency: 5 * time.Minute,
		ImageBufferSize:  64,
		ContainerTTL:     &defeaultContainerTTL,
	},
}
