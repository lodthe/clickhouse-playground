package main

import (
	"context"
	"os"
	"time"

	"clickhouse-playground/internal/dockertag"

	"github.com/aws/aws-sdk-go-v2/aws"
	gconfig "github.com/gookit/config/v2"
	gyaml "github.com/gookit/config/v2/yaml"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

const DefaultConfigPath = "config.yaml"

type RunnerType string

const (
	RunnerTypeEC2          RunnerType = "EC2"
	RunnerTypeDockerEngine RunnerType = "DOCKER_ENGINE"
)

type Config struct {
	LogLevel string `mapstructure:"log_level"`

	DockerImage DockerImage `mapstructure:"docker_image"`

	API API `mapstructure:"api"`

	PrometheusExportAddress string `mapstructure:"prometheus_address"`

	AWS AWS `mapstructure:"aws"`

	CustomConfigPath *string `mapstructure:"custom_config_path"`

	Runner Runner `mapstructure:"runner"`
}

type DockerImage struct {
	Name                string        `mapstructure:"image"`
	OS                  string        `mapstructure:"os"`
	Architecture        string        `mapstructure:"architecture"`
	CacheExpirationTime time.Duration `mapstructure:"image_tags_cache_expiration_time"`
}

type API struct {
	ListeningAddress string        `mapstructure:"address"`
	ServerTimeout    time.Duration `mapstructure:"server_timeout"`
}

type AWS struct {
	AccessKeyID     string `mapstructure:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key"`
	Region          string `mapstructure:"region"`

	QueryRunsTableName string `mapstructure:"query_runs_table"`
}

type Runner struct {
	Type RunnerType `mapstructure:"type"`
	Name string     `mapstructure:"name"`

	EC2          *EC2          `mapstructure:"ec2"`
	DockerEngine *DockerEngine `mapstructure:"docker_engine"`
}

type EC2 struct {
	InstanceID string `mapstructure:"instance_id"`
}

type DockerEngine struct {
	GC *DockerEngineGC `mapstructure:"gc"`
}

type DockerEngineGC struct {
	TriggerFrequency time.Duration `mapstructure:"trigger_frequency"`

	ContainerTTL *time.Duration `mapstructure:"container_ttl"`

	ImageGCCountThreshold *uint `mapstructure:"image_count_threshold"`
	ImageBufferSize       uint  `mapstructure:"image_buffer_size"`
}

func (r *Runner) Validate() error {
	if r.Name == "" {
		return errors.New("runner.name is required")
	}

	switch r.Type {
	case RunnerTypeEC2:
		if r.EC2 == nil {
			return errors.New("runner.ec2 is required when runner.type is EC2")
		}
		if r.EC2.InstanceID == "" {
			return errors.New("runner.ec2.instance_id is required when runner.type is EC2")
		}

	case RunnerTypeDockerEngine:
		gc := r.DockerEngine.GC
		if gc == nil {
			break
		}

		if gc.TriggerFrequency == 0 {
			gc.TriggerFrequency = 1 * time.Minute
		}

	case "":
		return errors.New("runner.type is required")

	default:
		return errors.Errorf("unknown runner type %s (supported: %s, %s)", r.Type, RunnerTypeEC2, RunnerTypeDockerEngine)
	}

	return nil
}

func LoadConfig() (*Config, error) {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = DefaultConfigPath
	}

	gconfig.WithOptions(
		gconfig.ParseEnv,
		gconfig.Readonly,
		func(opts *gconfig.Options) {
			opts.DecoderConfig = &mapstructure.DecoderConfig{
				TagName:          "mapstructure",
				WeaklyTypedInput: true,
				DecodeHook:       mapstructure.StringToTimeDurationHookFunc(),
			}
		},
	)
	gconfig.AddDriver(gyaml.Driver)

	err := gconfig.LoadFiles(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load config")
	}

	cfg := new(Config)
	err = gconfig.BindStruct("", cfg)
	if err != nil {
		return nil, errors.Wrap(err, "config binding failed")
	}

	err = cfg.validate()
	if err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	return cfg, nil
}

// validate verifies the loaded config and sets default values for missed fields.
func (c *Config) validate() error {
	if c.LogLevel == "" {
		c.LogLevel = "debug"
	}

	if c.DockerImage.Name == "" {
		return errors.New("docker_image.name is required")
	}
	if c.DockerImage.OS == "" {
		return errors.New("docker_image.os is required")
	}
	if c.DockerImage.Architecture == "" {
		return errors.New("docker_image.architecture is required")
	}
	if c.DockerImage.CacheExpirationTime == 0 {
		c.DockerImage.CacheExpirationTime = dockertag.DefaultExpirationTime
	}

	if c.API.ListeningAddress == "" {
		c.API.ListeningAddress = ":9000"
	}
	if c.API.ServerTimeout == 0 {
		c.API.ServerTimeout = 60 * time.Second
	}

	if c.PrometheusExportAddress == "" {
		c.PrometheusExportAddress = ":2112"
	}

	if c.AWS.Region == "" {
		return errors.New("aws.region is required")
	}
	if c.AWS.QueryRunsTableName == "" {
		return errors.New("aws.query_runs_table is required")
	}

	err := c.Runner.Validate()
	if err != nil {
		return errors.Wrap(err, "runner validation")
	}

	return nil
}

func (c *Config) Retrieve(_ context.Context) (aws.Credentials, error) {
	return aws.Credentials{
		AccessKeyID:     c.AWS.AccessKeyID,
		SecretAccessKey: c.AWS.SecretAccessKey,
		Source:          "local config",
	}, nil
}
