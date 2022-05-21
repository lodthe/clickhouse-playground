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

	// During the container garbage collection, all containers created
	// before (time.Now() - ContainerTTL) will be removed.
	ContainerTTL *time.Duration

	// Image gc triggers when there are at least ImageGCCountThreshold downloaded chp images.
	// After the garbage collection, at most ImageBufferSize least recently tagged images will be left.
	ImageGCCountThreshold uint
	ImageBufferSize       uint
}

var defaultContainerTTL = 60 * time.Second
var defaultImageBufferSize = uint(30)

var DefaultLocalDockerConfig = Config{
	ExecRetryDelay: 200 * time.Millisecond,
	MaxExecRetries: 20,

	CustomConfigPath: nil,

	GC: &GCConfig{
		TriggerFrequency:      5 * time.Minute,
		ContainerTTL:          &defaultContainerTTL,
		ImageGCCountThreshold: defaultImageBufferSize * 2,
		ImageBufferSize:       defaultImageBufferSize,
	},
}