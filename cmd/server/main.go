package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"clickhouse-playground/internal/qrunner"
	"clickhouse-playground/internal/queryrun"
	"clickhouse-playground/pkg/dockerhub"
	"clickhouse-playground/pkg/dockertag"
	api "clickhouse-playground/pkg/restapi"

	awsconf "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
)

const chServerImageName = "yandex/clickhouse-server"
const shutdownTimeout = 5 * time.Second
const tableName = "QueryRuns"

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zlog.Logger = zlog.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	awsRegion := os.Getenv("AWS_REGION")
	awsInstanceID := os.Getenv("AWS_INSTANCE_ID")
	bindAddress := os.Getenv("BIND_ADDRESS")
	if bindAddress == "" {
		bindAddress = ":9000"
	}

	ctx, cancel := context.WithCancel(context.Background())
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	cfg, err := awsconf.LoadDefaultConfig(ctx, awsconf.WithRegion(awsRegion))
	if err != nil {
		zlog.Fatal().Err(err).Msg("failed to load AWS config")
	}

	dynamodbClient := dynamodb.NewFromConfig(cfg)
	runRepo := queryrun.NewRepository(ctx, dynamodbClient, tableName)

	runner := qrunner.NewEC2(ctx, cfg, awsInstanceID)

	dockerhubCli := dockerhub.NewClient()
	tagStorage := dockertag.NewStorage(time.Minute, dockerhubCli)

	router := api.NewRouter(runner, tagStorage, runRepo, chServerImageName, 60*time.Second)

	zlog.Info().Str("address", bindAddress).Msg("starting the server")

	srv := &http.Server{
		Addr:    bindAddress,
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
