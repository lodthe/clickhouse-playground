package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lodthe/clickhouse-playground/internal/dockertag"
	"github.com/lodthe/clickhouse-playground/internal/qrunner"
	"github.com/lodthe/clickhouse-playground/internal/qrunner/coordinator"
	"github.com/lodthe/clickhouse-playground/internal/qrunner/dockerengine"
	"github.com/lodthe/clickhouse-playground/internal/queryrun"
	"github.com/lodthe/clickhouse-playground/pkg/dockerhub"
	api "github.com/lodthe/clickhouse-playground/pkg/restapi"

	awsconf "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
)

const shutdownTimeout = 5 * time.Second

func main() {
	// Listen to termination signals.
	ctx, cancel := context.WithCancel(context.Background())
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Initialize config.
	config, err := LoadConfig()
	if err != nil {
		zlog.Fatal().Err(err).Msg("config cannot be loaded")
	}

	// Initialize logger.
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	if config.LogFormat == PrettyLogFormat {
		zlog.Logger = zlog.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	lvl, err := zerolog.ParseLevel(config.LogLevel)
	if err != nil {
		zlog.Fatal().Err(err).Msg("invalid log level")
	}

	zlog.Logger = zlog.Logger.Level(lvl)
	logger := zlog.Logger

	// Load AWS credentials.
	var awsOpts []func(*awsconf.LoadOptions) error
	if config.AWS.AccessKeyID != "" {
		// Load AWS config with credentials when AccessKeyID is not empty.
		// Otherwise, we let SDK to pick credentials from available sources automatically.
		awsOpts = append(awsOpts, awsconf.WithCredentialsProvider(config))
	}

	awsOpts = append(awsOpts, awsconf.WithRegion(config.AWS.Region))

	awsConfig, err := awsconf.LoadDefaultConfig(ctx, awsOpts...)
	if err != nil {
		zlog.Fatal().Err(err).Msg("failed to load AWS config")
	}

	// Initialize storages.
	dynamodbClient := dynamodb.NewFromConfig(awsConfig)
	dockerhubCli := dockerhub.NewClient(dockerhub.DockerHubURL, dockerhub.DefaultMaxRPS, dockerhub.Auth(config.DockerImage.Auth))
	tagStorage := dockertag.NewCache(ctx, dockertag.Config{
		Repositories:   config.DockerImage.Repositories,
		OS:             config.DockerImage.OS,
		Architecture:   config.DockerImage.Architecture,
		ExpirationTime: config.DockerImage.CacheExpirationTime,
	}, logger, dockerhubCli)
	tagStorage.RunBackgroundUpdate()

	// Create runners and the coordinator.
	runners := initializeRunners(ctx, config, tagStorage, logger)

	coordinatorCfg := coordinator.Config{
		HealthChecksEnabled:   true,
		HealthCheckRetryDelay: config.Coordinator.HealthCheckRetryDelay,
	}
	coord := coordinator.New(ctx, logger, runners, coordinatorCfg)
	go func() {
		err := coord.Start()
		if err != nil {
			zlog.Fatal().Err(err).Msg("coordinator cannot be started")
		}
	}()

	// Initialize the REST server.
	runRepo := queryrun.NewRepository(ctx, dynamodbClient, config.AWS.QueryRunsTableName)

	lim := config.Limits
	router := api.NewRouter(api.RouterOpts{
		Logger:          logger,
		Runner:          coord,
		TagStorage:      tagStorage,
		RunRepo:         runRepo,
		Timeout:         config.API.ServerTimeout,
		MaxQueryLength:  lim.MaxOutputLength,
		MaxOutputLength: lim.MaxOutputLength,
	})

	srv := &http.Server{
		Addr:              config.API.ListeningAddress,
		Handler:           router,
		ReadTimeout:       20 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	go func() {
		zlog.Info().Str("address", config.API.ListeningAddress).Msg("starting the server")

		err := srv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			zlog.Fatal().Err(err).Msg("server listen failed")
		}
	}()

	// Export Prometheus metrics.
	go func() {
		zlog.Info().Str("address", config.PrometheusExportAddress).Msg("starting the prometheus exporter")

		metricSrv := &http.Server{
			Addr:              config.PrometheusExportAddress,
			Handler:           http.DefaultServeMux,
			ReadTimeout:       10 * time.Second,
			ReadHeaderTimeout: 5 * time.Second,
		}

		http.DefaultServeMux.Handle("/metrics", promhttp.Handler())
		err := metricSrv.ListenAndServe()
		if err != nil {
			zlog.Error().Err(err).Msg("prometheus exporter failed")
		}
	}()

	<-stop
	cancel()

	shutdownCtx, shutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdown()

	err = coord.Stop(shutdownCtx)
	if err != nil {
		zlog.Err(err).Msg("coordinator cannot be stopped")
	}

	err = srv.Shutdown(shutdownCtx)
	if err != nil {
		zlog.Error().Err(err).Msg("server shutdown failed")
	}
}

func initializeRunners(ctx context.Context, config *Config, tagStorage *dockertag.Cache, logger zerolog.Logger) []*coordinator.Runner {
	var runners []*coordinator.Runner
	for _, r := range config.Runners {
		var runner qrunner.Runner
		switch r.Type {
		case RunnerTypeDockerEngine:
			rcfg := dockerengine.DefaultConfig
			rcfg.DaemonURL = r.DockerEngine.DaemonURL
			rcfg.CustomConfigPath = r.DockerEngine.CustomConfigPath
			rcfg.QuotasPath = r.DockerEngine.QuotasPath
			rcfg.GC = nil

			if config.Settings.DefaultFormat != nil {
				rcfg.DefaultOutputFormat = *config.Settings.DefaultFormat
			}

			gc := r.DockerEngine.GC
			if gc != nil {
				rcfg.GC = &dockerengine.GCConfig{
					TriggerFrequency:      gc.TriggerFrequency,
					ContainerTTL:          gc.ContainerTTL,
					ImageGCCountThreshold: gc.ImageGCCountThreshold,
					ImageBufferSize:       gc.ImageBufferSize,
				}
			}

			rcfg.Container = dockerengine.ContainerSettings{
				NetworkMode: r.DockerEngine.Container.NetworkMode,
				CPULimit:    uint64(r.DockerEngine.Container.CPULimit * 1e9), // cpu -> nano cpu.
				CPUSet:      r.DockerEngine.Container.CPUSet,
				MemoryLimit: uint64(r.DockerEngine.Container.MemoryLimitMB * 1e6), // mb -> bytes.
			}

			if r.DockerEngine.Prewarm != nil && r.DockerEngine.Prewarm.MaxWarmContainers != nil {
				rcfg.MaxWarmContainers = *r.DockerEngine.Prewarm.MaxWarmContainers
			}

			var err error
			runner, err = dockerengine.New(ctx, logger, r.Name, rcfg, tagStorage)
			if err != nil {
				zlog.Fatal().Err(err).Msg("failed to create docker engine runner")
			}

		default:
			zlog.Fatal().Msg("invalid runner type")
		}

		runners = append(runners, coordinator.NewRunner(runner, r.Weight, r.MaxConcurrency))
	}

	return runners
}
