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
	"clickhouse-playground/internal/qrunner/coordinator"
	"clickhouse-playground/internal/qrunner/dockerengine"
	"clickhouse-playground/internal/qrunner/ec2"
	"clickhouse-playground/internal/queryrun"
	"clickhouse-playground/pkg/dockerhub"
	api "clickhouse-playground/pkg/restapi"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconf "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/docker/cli/cli/connhelper"
	dockercli "github.com/docker/docker/client"
	"github.com/pkg/errors"
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

	lvl, err := zerolog.ParseLevel(config.LogLevel)
	if err != nil {
		zlog.Fatal().Err(err).Msg("invalid log level")
	}

	zlog.Logger = zlog.Logger.Level(lvl)

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

	// Create runners.
	var runners []*coordinator.Runner
	for _, r := range config.Runners {
		var runner qrunner.Runner
		switch r.Type {
		case RunnerTypeEC2:
			runner = newEC2Runner(ctx, r, awsConfig)

		case RunnerTypeDockerEngine:
			runner, err = newDockerEngineRunner(ctx, config.DockerImage, r, tagStorage)
			if err != nil {
				zlog.Fatal().Err(err).Msg("failed to create docker engine runner")
			}

		default:
			zlog.Fatal().Msg("invalid runner")
		}

		runners = append(runners, coordinator.NewRunner(runner, r.Weight))
	}

	coord := coordinator.New(ctx, zlog.Logger, runners)
	go func() {
		err := coord.Start()
		if err != nil {
			zlog.Fatal().Err(err).Msg("runner cannot be started")
		}
	}()

	runRepo := queryrun.NewRepository(ctx, dynamodbClient, config.AWS.QueryRunsTableName)

	router := api.NewRouter(coord, tagStorage, runRepo, config.DockerImage.Name, config.API.ServerTimeout)

	// Start REST API server.
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

	// Export Prometheus metrics.
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

	err = coord.Stop()
	if err != nil {
		zlog.Err(err).Msg("coordinator cannot be stopped")
	}

	err = srv.Shutdown(shutdownCtx)
	if err != nil {
		zlog.Error().Err(err).Msg("server shutdown failed")
	}
}

func newEC2Runner(ctx context.Context, config Runner, awsConfig aws.Config) *ec2.Runner {
	return ec2.New(ctx, zlog.Logger, config.Name, ec2.DefaultConfig, awsConfig, config.EC2.InstanceID)
}

func newDockerEngineRunner(ctx context.Context, img DockerImage, config Runner, tagStorage *dockertag.Cache) (*dockerengine.Runner, error) {
	opts, err := getDockerEngineOpts(config.DockerEngine)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build options for Docker client")
	}

	dockerCli, err := dockercli.NewClientWithOpts(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Docker client")
	}

	ping, err := dockerCli.Ping(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to ping the docker daemon of %s runner", config.Name)
	}

	zlog.Info().
		Str("runner_name", config.Name).
		Str("api_version", ping.APIVersion).
		Msg("established a connection with a docker daemon")

	localCfg := dockerengine.DefaultConfig
	localCfg.CustomConfigPath = config.DockerEngine.CustomConfigPath
	localCfg.Repository = img.Name
	localCfg.GC = nil

	localCfg.Container = dockerengine.ContainerResources{
		CPULimit:    uint64(config.DockerEngine.ContainerResources.CPULimit * 1e9), // cpu -> nano cpu.
		CPUSet:      config.DockerEngine.ContainerResources.CPUSet,
		MemoryLimit: uint64(config.DockerEngine.ContainerResources.MemoryLimitMB * 1e6), // mb -> bytes.
	}

	gc := config.DockerEngine.GC
	if gc != nil {
		localCfg.GC = &dockerengine.GCConfig{
			TriggerFrequency:      gc.TriggerFrequency,
			ContainerTTL:          gc.ContainerTTL,
			ImageGCCountThreshold: gc.ImageGCCountThreshold,
			ImageBufferSize:       gc.ImageBufferSize,
		}
	}

	return dockerengine.New(ctx, zlog.Logger, config.Name, localCfg, dockerCli, tagStorage), nil
}

func getDockerEngineOpts(config *DockerEngine) ([]dockercli.Opt, error) {
	opts := []dockercli.Opt{
		dockercli.WithAPIVersionNegotiation(),
	}

	if config.DaemonURL == nil {
		return opts, nil
	}

	// Set 'StrictHostKeyChecking=no' to simplify startup in Docker containers.
	helper, err := connhelper.GetConnectionHelperWithSSHOpts(*config.DaemonURL, []string{"-o", "StrictHostKeyChecking=no"})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create ssh connection")
	}
	if helper == nil {
		return nil, errors.Wrap(err, "provided daemon_url cannot be recognized by Docker lib")
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: helper.Dialer,
		},
	}

	opts = append(opts,
		dockercli.WithHTTPClient(httpClient),
		dockercli.WithHost(helper.Host),
		dockercli.WithDialContext(helper.Dialer),
	)

	return opts, nil
}
