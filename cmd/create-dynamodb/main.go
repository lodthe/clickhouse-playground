package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func main() {
	awsRegion := os.Getenv("AWS_REGION")

	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(awsRegion))
	if err != nil {
		log.Fatalf("config load failed: %v\n", err)
	}

	client := dynamodb.NewFromConfig(cfg)

	tableName := "QueryRuns"

	param := &dynamodb.CreateTableInput{
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("Id"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("Id"),
				KeyType:       types.KeyTypeHash,
			},
		},
		ProvisionedThroughput: &types.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(5),
			WriteCapacityUnits: aws.Int64(5),
		},
		TableName:  aws.String(tableName),
		TableClass: types.TableClassStandardInfrequentAccess,
	}

	_, err = client.CreateTable(context.TODO(), param)
	if err != nil {
		log.Fatalf("table creation failed: %s\n", err)
	}

	log.Printf("table was created\n")
}
