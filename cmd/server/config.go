package main

import (
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/pkg/errors"
)

type Runner string

const (
	RunnerEC2         Runner = "EC2"
	RunnerLocalDocker Runner = "LOCAL_DOCKER"
)

type Config struct {
	DockerImage DockerImage

	ListeningAddress string        `env:"LISTENING_ADDRESS" envDefault:":9000"`
	ServerTimeout    time.Duration `env:"SERVER_TIMEOUT" envDefault:"60s"`

	AWSAuth               AWSAuth
	AWSQueryRunsTableName string `env:"AWS_QUERY_RUNS_TABLE_NAME,required"`

	CustomConfigPath *string `env:"CUSTOM_CONFIG_PATH"`

	Runner      Runner `env:"RUNNER,required" envDefault:"EC2"`
	EC2         *EC2
	LocalDocker *LocalDocker
}

type DockerImage struct {
	Name          string        `env:"DOCKER_IMAGE_NAME,required" envDefault:"clickhouse/clickhouse-server"`
	OS            string        `env:"DOCKER_IMAGE_OS" envDefault:"linux"`
	Architecture  string        `env:"DOCKER_IMAGE_ARCHITECTURE" envDefault:"amd64"`
	CacheLifetime time.Duration `env:"DOCKER_IMAGE_TAG_CACHE_LIFETIME" envDefault:"3m"`
}

type AWSAuth struct {
	SecretAccessKey string `env:"AWS_SECRET_ACCESS_KEY,required"`
	AccessKeyID     string `env:"AWS_ACCESS_KEY_ID,required"`
	Region          string `env:"AWS_REGION,required"`
}

type EC2 struct {
	AWSInstanceID string `env:"AWS_INSTANCE_ID,required"`
}

type LocalDocker struct {
}

func LoadConfig() (*Config, error) {
	cfg := new(Config)
	err := env.Parse(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "config load")
	}

	switch cfg.Runner {
	case RunnerEC2:
		cfg.EC2 = new(EC2)
		err = env.Parse(cfg.EC2)
		if err != nil {
			return nil, errors.Wrap(err, "EC2 runner config load")
		}

	case RunnerLocalDocker:
		cfg.LocalDocker = new(LocalDocker)
		err = env.Parse(cfg.LocalDocker)
		if err != nil {
			return nil, errors.Wrap(err, "LocalDocker runner config load")
		}

	default:
		return nil, errors.New("unknown runner type")
	}

	return cfg, nil
}
