package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"clickhouse-playground/internal/dockertag"
	"clickhouse-playground/internal/qrunner"
	"clickhouse-playground/internal/queryrun"
	"clickhouse-playground/pkg/dockerhub"
	api "clickhouse-playground/pkg/restapi"

	awsconf "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dockercli "github.com/docker/docker/client"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
)

const shutdownTimeout = 5 * time.Second

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zlog.Logger = zlog.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	config, err := LoadConfig()
	if err != nil {
		zlog.Fatal().Err(err).Msg("config cannot be loaded")
	}

	ctx, cancel := context.WithCancel(context.Background())
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	awsConfig, err := awsconf.LoadDefaultConfig(
		ctx,
		awsconf.WithCredentialsProvider(config),
		awsconf.WithRegion(config.AWS.Region),
	)
	if err != nil {
		zlog.Fatal().Err(err).Msg("failed to load AWS config")
	}

	dynamodbClient := dynamodb.NewFromConfig(awsConfig)

	dockerhubCli := dockerhub.NewClient(dockerhub.DockerHubURL, dockerhub.DefaultMaxRPS)
	tagStorage := dockertag.NewCache(ctx, dockertag.Config{
		Image:          config.DockerImage.Name,
		OS:             config.DockerImage.OS,
		Architecture:   config.DockerImage.Architecture,
		ExpirationTime: config.DockerImage.CacheExpirationTime,
	}, dockerhubCli)
	tagStorage.RunBackgroundUpdate()

	var runner qrunner.Runner
	switch config.Runner.Type {
	case RunnerTypeEC2:
		runner = qrunner.NewEC2(ctx, awsConfig, config.DockerImage.Name, config.Runner.EC2.InstanceID)

	case RunnerTypeLocalDocker:
		dockerCli, err := dockercli.NewClientWithOpts(dockercli.WithAPIVersionNegotiation())
		if err != nil {
			zlog.Fatal().Err(err).Msg("failed to create docker engine client")
		}

		localCfg := qrunner.DefaultLocalDockerConfig
		localCfg.CustomConfigPath = config.CustomConfigPath

		runner = qrunner.NewLocalDocker(ctx, localCfg, dockerCli, config.DockerImage.Name, tagStorage)

	default:
		zlog.Fatal().Msg("invalid runner")
	}

	runRepo := queryrun.NewRepository(ctx, dynamodbClient, config.AWS.QueryRunsTableName)

	router := api.NewRouter(runner, tagStorage, runRepo, config.DockerImage.Name, config.API.ServerTimeout)

	srv := &http.Server{
		Addr:    config.API.ListeningAddress,
		Handler: router,
	}
	go func() {
		zlog.Info().Str("address", config.API.ListeningAddress).Msg("starting the server")

		err := srv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			zlog.Fatal().Err(err).Msg("server listen failed")
		}
	}()

	go func() {
		zlog.Info().Str("address", config.PrometheusExportAddress).Msg("starting the prometheus exporter")

		http.Handle("/metrics", promhttp.Handler())
		err := http.ListenAndServe(config.PrometheusExportAddress, nil)
		if err != nil {
			zlog.Error().Err(err).Msg("prometheus exporter failed")
		}
	}()

	<-stop
	cancel()

	shutdownCtx, shutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdown()

	err = srv.Shutdown(shutdownCtx)
	if err != nil {
		zlog.Error().Err(err).Msg("server shutdown failed")
	}
}
