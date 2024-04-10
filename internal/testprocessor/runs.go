package testprocessor

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconf "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	gconfig "github.com/gookit/config/v2"
	"github.com/pkg/errors"
	zlog "github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

const clickhouseDatabase = "clickhouse"

type ImportRunsParams struct {
	AwsAccessKeyID        string
	AwsSecretAccessKey    string
	AwsRegion             string
	AwsQueryRunsTableName string

	RunsBefore time.Time
	RunsAfter  time.Time

	OutputPath string
}

func (p *ImportRunsParams) Retrieve(_ context.Context) (aws.Credentials, error) {
	return aws.Credentials{
		AccessKeyID:     p.AwsAccessKeyID,
		SecretAccessKey: p.AwsSecretAccessKey,
		Source:          "local config",
	}, nil
}

type RunsInput struct {
	Runs []*Run `mapstructure:"runs" yaml:"runs"`
}

type Run struct {
	Database    string        `mapstructure:"database" yaml:"database" dynamodbav:"Database"`
	Version     string        `mapstructure:"version" yaml:"version" dynamodbav:"Version"`
	Query       string        `mapstructure:"query" yaml:"query" dynamodbav:"Input"`
	Timestamp   time.Time     `mapstructure:"timestamp" yaml:"timestamp" dynamodbav:"CreatedAt"`
	TimeElapsed time.Duration `mapstructure:"-" yaml:"-" dynamodbav:"-"`
}

func (r *Run) CsvRow() []string {
	return []string{r.Database, r.Version, r.Query, r.TimeElapsed.String()}
}

func loadRuns(runsFilePath string) (*RunsInput, error) {
	err := gconfig.LoadFiles(runsFilePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load runs")
	}

	runs := new(RunsInput)
	err = gconfig.BindStruct("", runs)
	if err != nil {
		return nil, errors.Wrap(err, "runs data binding failed")
	}

	runs.validate()

	return runs, nil
}

// validate verifies the loaded runs and sets default values for missed fields.
func (runs *RunsInput) validate() {
	for _, run := range runs.Runs {
		if run.Database == "" {
			run.Database = clickhouseDatabase
		}
	}
}

func ImportRunsFromAWS(params *ImportRunsParams) {
	ctx := context.Background()

	var awsOpts []func(*awsconf.LoadOptions) error
	if params.AwsAccessKeyID != "" {
		awsOpts = append(awsOpts, awsconf.WithCredentialsProvider(params))
	}

	awsOpts = append(awsOpts, awsconf.WithRegion(params.AwsRegion))

	awsConfig, err := awsconf.LoadDefaultConfig(ctx, awsOpts...)
	if err != nil {
		zlog.Fatal().Err(err).Msg("failed to load AWS config")
	}

	outputFile, err := os.Create(params.OutputPath)
	if err != nil {
		zlog.Fatal().Err(err).Msg("failed create output file")
	}
	defer outputFile.Close()

	dynamodbClient := dynamodb.NewFromConfig(awsConfig)

	var importedRuns RunsInput
	var lastEvaluatedKey map[string]types.AttributeValue

	for {
		out, scanErr := dynamodbClient.Scan(ctx, &dynamodb.ScanInput{
			TableName:         aws.String(params.AwsQueryRunsTableName),
			ExclusiveStartKey: lastEvaluatedKey,
			FilterExpression: aws.String("CreatedAt >= :createdAfter AND CreatedAt <= :createdBefore AND " +
				"((attribute_exists(#database) AND #database = :clickhouseDatabase) OR attribute_not_exists(#database))"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":createdBefore":      &types.AttributeValueMemberS{Value: params.RunsBefore.Format(time.RFC3339Nano)},
				":createdAfter":       &types.AttributeValueMemberS{Value: params.RunsAfter.Format(time.RFC3339Nano)},
				":clickhouseDatabase": &types.AttributeValueMemberS{Value: clickhouseDatabase},
			},
			ExpressionAttributeNames: map[string]string{
				"#database": "Database",
			},
		})

		if scanErr != nil {
			zlog.Fatal().Err(scanErr).Msg("error while scan")
		}

		lastEvaluatedKey = out.LastEvaluatedKey

		for _, item := range out.Items {
			run := new(Run)

			err = attributevalue.UnmarshalMap(item, run)
			if err != nil {
				zlog.Error().Err(err).Msg("failed to unmarshal db run")
				continue
			}

			if run.Database == "" {
				run.Database = clickhouseDatabase
			}

			importedRuns.Runs = append(importedRuns.Runs, run)
		}

		if len(lastEvaluatedKey) == 0 {
			break
		}
	}

	sort.Slice(importedRuns.Runs, func(i, j int) bool {
		return importedRuns.Runs[i].Timestamp.Before(importedRuns.Runs[j].Timestamp)
	})

	yamlFile, err := yaml.Marshal(&importedRuns)
	if err != nil {
		zlog.Error().Err(err).Msg("failed to marshal runs to yaml")
	}

	_, err = outputFile.WriteString(string(yamlFile))
	if err != nil {
		zlog.Error().Err(err).Msg("failed to write data to output file")
	}

	fmt.Printf("Successfyly imported %d runs\n", len(importedRuns.Runs))
}
