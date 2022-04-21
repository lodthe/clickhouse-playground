package queryrun

import (
	"context"

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
	err = attributevalue.UnmarshalMap(out.Item, run)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	if run.ID == "" {
		return nil, ErrNotFound
	}

	return run, nil
}
