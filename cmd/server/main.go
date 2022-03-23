package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"clickhouse-playground/internal/runner"
	"clickhouse-playground/pkg/dockerhub"
	"clickhouse-playground/pkg/dockertag"
	api "clickhouse-playground/pkg/restapi"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
)

const chServerImageName = "yandex/clickhouse-server"
const shutdownTimeout = 5 * time.Second

func main() {
	awsRegion := os.Getenv("AWS_REGION")
	awsInstanceID := os.Getenv("AWS_INSTANCE_ID")
	bindAddress := os.Getenv("BIND_ADDRESS")
	if bindAddress == "" {
		bindAddress = ":9000"
	}

	ctx, cancel := context.WithCancel(context.Background())
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(awsRegion),
		Credentials: credentials.NewEnvCredentials(),
	})
	if err != nil {
		log.Fatalf("session was not created: %v\n", err)
	}

	run := runner.NewEC2(ctx, sess, awsInstanceID)

	dockerhubCli := dockerhub.NewClient()
	tagStorage := dockertag.NewStorage(time.Minute, dockerhubCli)

	router := api.NewRouter(run, tagStorage, chServerImageName, 60*time.Second)

	srv := &http.Server{
		Addr:    bindAddress,
		Handler: router,
	}
	go func() {
		err = srv.ListenAndServe()
		if err != nil {
			log.Fatalf("listen failed: %v\n", err)
		}
	}()

	<-stop
	cancel()

	shutdownCtx, _ := context.WithTimeout(ctx, shutdownTimeout)

	err = srv.Shutdown(shutdownCtx)
	if err != nil {
		log.Printf("server shutdown failed: %v\n", err)
	}
}
