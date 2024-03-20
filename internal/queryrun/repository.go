package queryrun

import (
	"context"

	"clickhouse-playground/internal/database"
	"clickhouse-playground/internal/database/runsettings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pkg/errors"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	Create(run *Run) error
	Get(id string) (*Run, error)
}

type Repo struct {
	ctx    context.Context
	client *dynamodb.Client

	tableName *string
}

func NewRepository(ctx context.Context, client *dynamodb.Client, tableName string) *Repo {
	return &Repo{
		ctx:       ctx,
		client:    client,
		tableName: aws.String(tableName),
	}
}

func (r *Repo) Create(run *Run) error {
	marshaled, err := attributevalue.MarshalMap(run)
	if err != nil {
		return errors.Wrap(err, "marshal failed")
	}

	_, err = r.client.PutItem(r.ctx, &dynamodb.PutItemInput{
		TableName: r.tableName,
		Item:      marshaled,
	})
	if err != nil {
		return errors.Wrap(err, "put failed")
	}

	return nil
}

func (r *Repo) Get(id string) (*Run, error) {
	out, err := r.client.GetItem(r.ctx, &dynamodb.GetItemInput{
		TableName: r.tableName,
		Key: map[string]types.AttributeValue{
			"Id": &types.AttributeValueMemberS{Value: id},
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "get failed")
	}

	run := new(Run)

	// Done because UnmarshalMap can't unmarshal in interface{}
	var databaseType database.Type
	attributevalue.Unmarshal(out.Item["Database"], &databaseType)
	// TODO: add switch-case in the future
	if databaseType == database.TypeClickHouse {
		run.Settings = &runsettings.ClickHouseSettings{}
	}

	err = attributevalue.UnmarshalMap(out.Item, run)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	if run.ID == "" {
		return nil, ErrNotFound
	}

	return run, nil
}
