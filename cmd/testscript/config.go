package main

import (
	"clickhouse-playground/internal/testprocessor"

	gconfig "github.com/gookit/config/v2"
	gyaml "github.com/gookit/config/v2/yaml"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"

	"os"
	"time"
)

const DefaultConfigPath = "test_script_config.yml"
const DefaultRunsDataPath = "runs_data.yml"
const DefaultOutputPath = "output.csv"

type Config struct {
	Playground PlaygroundSettings `mapstructure:"playground"`
	TestScript TestScript         `mapstructure:"test_script"`
	AWS        AWS                `mapstructure:"aws"`
}

type PlaygroundSettings struct {
	BaseURL string `mapstructure:"base_url"`
}

type TestScript struct {
	Mode         testprocessor.Mode `mapstructure:"mode"`
	RunsDataPath string             `mapstructure:"runs_data_path"`
	OutputPath   *string            `mapstructure:"output_path"`
	DefaultQuery *string            `mapstructure:"default_query"`
	Percentiles  []int              `mapstructure:"percentiles_to_calculate"`
}

type AWS struct {
	AccessKeyID        string `mapstructure:"access_key_id"`
	SecretAccessKey    string `mapstructure:"secret_access_key"`
	Region             string `mapstructure:"region"`
	QueryRunsTableName string `mapstructure:"query_runs_table"`
}

func LoadConfig() (*Config, error) {
	path := os.Getenv("TEST_SCRIPT_CONFIG_PATH")
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
				DecodeHook:       mapstructure.StringToTimeHookFunc(time.RFC3339Nano),
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
	if c.Playground.BaseURL == "" {
		return errors.New("missing playground base url")
	}

	if c.TestScript.DefaultQuery != nil && *c.TestScript.DefaultQuery == "" {
		return errors.New("default query cannot be empty")
	}

	if c.TestScript.RunsDataPath == "" {
		c.TestScript.RunsDataPath = DefaultRunsDataPath
	}

	if c.TestScript.OutputPath == nil {
		outputPath := DefaultOutputPath
		c.TestScript.OutputPath = &outputPath
	}

	return nil
}
