package ec2

import "time"

type Config struct {
	Repository string

	// If this time is reached and the command hasn't already started running, it won't run. Measured in seconds.
	SSMCommandWaitTimeout int32

	SendQueryRetryDelay time.Duration
	MaxSendQueryRetries int

	WaitCommandExecutionDelay time.Duration
}

var DefaultConfig = Config{
	Repository:                "clickhouse/clickhouse-server",
	SSMCommandWaitTimeout:     30,
	SendQueryRetryDelay:       300 * time.Millisecond,
	MaxSendQueryRetries:       20,
	WaitCommandExecutionDelay: 50 * time.Millisecond,
}
