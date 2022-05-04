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

	awsConfig, err := awsconf.LoadDefaultConfig(ctx, awsconf.WithRegion(config.AWSAuth.Region))
	if err != nil {
		zlog.Fatal().Err(err).Msg("failed to load AWS config")
	}

	dynamodbClient := dynamodb.NewFromConfig(awsConfig)

	dockerhubCli := dockerhub.NewClient(dockerhub.DockerHubURL, dockerhub.DefaultMaxRPS)
	tagStorage := dockertag.NewStorage(dockertag.Config{
		Image:          config.DockerImage.Name,
		OS:             config.DockerImage.OS,
		Architecture:   config.DockerImage.Architecture,
		ExpirationTime: config.DockerImage.CacheLifetime,
	}, dockerhubCli)

	var runner qrunner.Runner
	switch config.Runner {
	case RunnerEC2:
		runner = qrunner.NewEC2(ctx, awsConfig, config.DockerImage.Name, config.EC2.AWSInstanceID)

	case RunnerLocalDocker:
		dockerCli, err := dockercli.NewClientWithOpts(dockercli.WithAPIVersionNegotiation())
		if err != nil {
			zlog.Fatal().Err(err).Msg("failed to create docker engine client")
		}

		runner = qrunner.NewLocalDocker(ctx, dockerCli, config.DockerImage.Name, tagStorage)

	default:
		zlog.Fatal().Msg("invalid runner")
	}

	runRepo := queryrun.NewRepository(ctx, dynamodbClient, config.AWSQueryRunsTableName)

	router := api.NewRouter(runner, tagStorage, runRepo, config.DockerImage.Name, config.ServerTimeout)

	zlog.Info().Str("address", config.ListeningAddress).Msg("starting the server")

	srv := &http.Server{
		Addr:    config.ListeningAddress,
		Handler: router,
	}
	go func() {
		err := srv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			zlog.Fatal().Err(err).Msg("server listen failed")
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
