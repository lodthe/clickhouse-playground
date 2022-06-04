package dockerengine

import "time"

type Config struct {
	DaemonURL *string

	Repository string

	ExecRetryDelay time.Duration
	MaxExecRetries int

	// Path to the xml or yaml config which will be mounted to the ../config.d/ directory.
	CustomConfigPath *string

	// Path to the quotas config that will be mounted to the ../users.d/ directory.
	// https://clickhouse.com/docs/en/operations/quotas/
	QuotasPath *string

	GC *GCConfig

	StatusCollectionFrequency time.Duration

	Container ContainerResources
}

type ContainerResources struct {
	CPULimit    uint64 // In nano cpus (1 core = 1e9 nano cpus). If 0, then unlimited.
	CPUSet      string // A comma-separated list or hyphen-separated range of CPUs a container can use. If "", then any cores can be used.
	MemoryLimit uint64 // In bytes. If 0, then unlimited.
}

type GCConfig struct {
	// How often GC will be triggered.
	TriggerFrequency time.Duration

	// During the container garbage collection, all containers created
	// before (time.Now() - ContainerTTL) will be removed.
	// If ContainerTTL is nil, containers are not force removed.
	ContainerTTL *time.Duration

	// Image gc triggers when there are at least ImageGCCountThreshold downloaded chp images.
	// After the garbage collection, at most ImageBufferSize least recently tagged images will be left.
	// If ImageGCCountThreshold is missed, images are not pruned.
	ImageGCCountThreshold *uint
	ImageBufferSize       uint
}

var defaultContainerTTL = 60 * time.Second
var defaultImageGCCountThreshold = uint(60)
var defaultImageBufferSize = uint(30)

var DefaultConfig = Config{
	Repository: "clickhouse/clickhouse-server",

	ExecRetryDelay: 200 * time.Millisecond,
	MaxExecRetries: 20,

	CustomConfigPath: nil,
	QuotasPath:       nil,

	GC: &GCConfig{
		TriggerFrequency:      5 * time.Minute,
		ContainerTTL:          &defaultContainerTTL,
		ImageGCCountThreshold: &defaultImageGCCountThreshold,
		ImageBufferSize:       defaultImageBufferSize,
	},

	StatusCollectionFrequency: 30 * time.Second,

	Container: ContainerResources{
		CPULimit:    2 * 1e9,
		CPUSet:      "",
		MemoryLimit: 1 * 1e9,
	},
}
