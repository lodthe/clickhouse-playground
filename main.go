package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"clickhouse-playground/internal/runner"
	"clickhouse-playground/pkg/dockerhub"
	"clickhouse-playground/pkg/dockertag"
	api "clickhouse-playground/pkg/restapi"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
)

func main() {
	ctx := context.Background()
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(os.Getenv("AWS_REGION")),
		Credentials: credentials.NewEnvCredentials(),
	})
	if err != nil {
		log.Fatalf("session is not created: %v", err)
	}

	run := runner.NewEC2(ctx, sess, os.Getenv("AWS_INSTANCE_ID"))

	chServerImageName := "yandex/clickhouse-server"

	dockerhubCli := dockerhub.NewClient()
	tagStorage := dockertag.NewStorage(time.Minute, dockerhubCli)

	router := api.NewRouter(run, tagStorage, chServerImageName, 60*time.Second)

	err = http.ListenAndServe(":9000", router)
	if err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
