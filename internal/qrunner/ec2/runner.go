package ec2

import (
	"context"
	"strings"
	"time"

	"clickhouse-playground/internal/qrunner"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/pkg/errors"
	zlog "github.com/rs/zerolog/log"
)

// Runner is a EC2 type runner that executes SQL queries on the specified Amazon EC2 instance via Amazon SSM.
//
// This runner creates and manages Docker containers by sending shell commands that trigger Docker CLI.
// The provided AWS config must be authorized to execute commands on the specified EC2 instance. Furthermore,
// you have to start the SSM daemon on the server.
type Runner struct {
	ctx context.Context
	cfg Config

	ssm        *ssm.Client
	instanceID string
}

func New(ctx context.Context, cfg Config, awsConfig aws.Config, instanceID string) *Runner {
	return &Runner{
		ctx:        ctx,
		cfg:        cfg,
		ssm:        ssm.NewFromConfig(awsConfig),
		instanceID: instanceID,
	}
}

func (r *Runner) StartGarbageCollector() {
	zlog.Info().Msg("gc is not implemented for the ec2 runner")
}

func (r *Runner) RunQuery(ctx context.Context, runID string, query string, version string) (string, error) {
	containerID, err := r.runContainer(ctx, version)
	if err != nil {
		return "", errors.Wrap(err, "failed to run container")
	}

	defer func() {
		err := r.killContainer(r.ctx, containerID)
		if err != nil {
			zlog.Err(err).
				Str("run_id", runID).
				Str("container_id", containerID).
				Msg("failed to kill container")
		}
	}()

	output, err := r.runQuery(ctx, containerID, query)
	if err != nil {
		return "", errors.Wrap(err, "failed to run query")
	}

	return output, nil
}

func (r *Runner) sendCommand(ctx context.Context, cmd string) (stdout string, stderr string, err error) {
	sendOutput, err := r.ssm.SendCommand(ctx, &ssm.SendCommandInput{
		DocumentName: aws.String("AWS-RunShellScript"),
		InstanceIds:  []string{r.instanceID},
		Parameters: map[string][]string{
			"commands": {cmd},
		},
		TimeoutSeconds: r.cfg.SSMCommandWaitTimeout,
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
			time.Sleep(r.cfg.WaitCommandExecutionDelay)
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

func (r *Runner) runContainer(ctx context.Context, version string) (string, error) {
	stdout, _, err := r.sendCommand(ctx, cmdRunContainer(r.cfg.Repository, version))
	if err != nil {
		return "", err
	}

	idx := strings.IndexRune(stdout, '\n')
	if idx == -1 {
		return "", errors.New("incompatible stdout format")
	}

	return stdout[:idx], nil
}

func (r *Runner) killContainer(ctx context.Context, id string) error {
	_, _, err := r.sendCommand(ctx, cmdKillContainer(id))

	return err
}

func (r *Runner) runQuery(ctx context.Context, containerID string, query string) (string, error) {
	var stdout string
	var stderr string
	var err error

	cmd := cmdRunQuery(containerID, query)

	for retry := 0; retry < r.cfg.MaxSendQueryRetries; retry++ {
		stdout, stderr, err = r.sendCommand(ctx, cmd)
		if err != nil {
			return "", err
		}

		if qrunner.CheckIfClickHouseIsReady(stderr) {
			break
		}

		time.Sleep(r.cfg.SendQueryRetryDelay)
	}

	if stderr == "" {
		return stdout, nil
	}

	return stdout + "\n" + stderr, nil
}
