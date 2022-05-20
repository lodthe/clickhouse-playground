package ec2

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/pkg/errors"
	zlog "github.com/rs/zerolog/log"
)

const (
	sendCommandTimeout = 30
)

// EC2 is a runner that executes SQL queries on the specified EC2 instance via Amazon SSM.
type EC2 struct {
	ctx context.Context
	ssm *ssm.Client

	imageName  string
	instanceID string
}

func NewEC2(ctx context.Context, cfg aws.Config, imageName string, instanceID string) *EC2 {
	return &EC2{
		ctx:        ctx,
		ssm:        ssm.NewFromConfig(cfg),
		imageName:  imageName,
		instanceID: instanceID,
	}
}

func (r *EC2) StartGarbageCollector() {
	zlog.Info().Msg("gc is not implemented for the ec2 runner")
}

func (r *EC2) RunQuery(ctx context.Context, runID string, query string, version string) (string, error) {
	containerID, err := r.runContainer(ctx, version)
	if err != nil {
		return "", errors.Wrap(err, "failed to run container")
	}

	output, err := r.runQuery(ctx, containerID, query)
	if err != nil {
		return "", errors.Wrap(err, "failed to run query")
	}

	err = r.killContainer(ctx, containerID)
	if err != nil {
		return "", errors.Wrap(err, "failed to kill container")
	}

	return output, nil
}

func (r *EC2) sendCommand(ctx context.Context, cmd string) (stdout string, stderr string, err error) {
	sendOutput, err := r.ssm.SendCommand(ctx, &ssm.SendCommandInput{
		DocumentName: aws.String("AWS-RunShellScript"),
		InstanceIds:  []string{r.instanceID},
		Parameters: map[string][]string{
			"commands": {cmd},
		},
		TimeoutSeconds: sendCommandTimeout,
	})
	if err != nil {
		return "", "", errors.Wrap(err, "send failed")
	}

	if sendOutput.Command == nil {
		return "", "", errors.New("missed command")
	}

	zlog.Debug().Str("id", *sendOutput.Command.CommandId).Str("command", cmd).Msg("sent a command to SSM")

	for {
		invocation, err := r.ssm.GetCommandInvocation(ctx, &ssm.GetCommandInvocationInput{
			CommandId:  sendOutput.Command.CommandId,
			InstanceId: aws.String(r.instanceID),
		})
		if err != nil {
			var invocationDoesNotExist *ssmtypes.InvocationDoesNotExist
			if errors.As(err, &invocationDoesNotExist) {
				zlog.Debug().Str("id", *sendOutput.Command.CommandId).Msg("invocation doesn't exist")
				time.Sleep(50 * time.Millisecond)

				continue
			}

			return "", "", errors.Wrap(err, "failed to get detailed description")
		}

		if invocation.Status == ssmtypes.CommandInvocationStatusInProgress {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		if invocation.StandardOutputContent != nil {
			stdout = *invocation.StandardOutputContent
		}
		if invocation.StandardErrorContent != nil {
			stderr = *invocation.StandardErrorContent
		}

		return stdout, stderr, nil
	}
}

func (r *EC2) runContainer(ctx context.Context, clickhouseVersion string) (string, error) {
	// TODO: Fix injection.
	cmd := fmt.Sprintf("docker run -d --ulimit nofile=262144:262144 -p 8123 %s:%s", r.imageName, clickhouseVersion)
	stdout, _, err := r.sendCommand(ctx, cmd)
	if err != nil {
		return "", err
	}

	idx := strings.IndexRune(stdout, '\n')
	if idx == -1 {
		return "", errors.New("incompatible stdout format")
	}

	return stdout[:idx], nil
}

func (r *EC2) killContainer(ctx context.Context, id string) error {
	// TODO: Fix injection.
	cmd := fmt.Sprintf("docker kill %s", id)
	_, _, err := r.sendCommand(ctx, cmd)

	return err
}

func (r *EC2) runQuery(ctx context.Context, containerID string, query string) (string, error) {
	// TODO: Fix injection.
	var stdout string
	var stderr string
	var err error

	query = strings.ReplaceAll(query, "\"", "\\\"")
	cmd := fmt.Sprintf("docker exec %s clickhouse-client -n -m --query \"%s\"", containerID, query) // nolint

	for retry := 0; retry < 15; retry++ {
		stdout, stderr, err = r.sendCommand(ctx, cmd)
		if err != nil {
			return "", err
		}

		if !strings.Contains(stderr, "DB::NetException: Connection refused") {
			break
		}

		time.Sleep(300 * time.Millisecond)
	}

	if stderr == "" {
		return stdout, nil
	}

	return stdout + "\n" + stderr, nil
}
