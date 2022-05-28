package qrunner

import "strings"

// CheckIfClickHouseIsReady checks whether a ClickHouse instance has accepted a query.
// There are no mechanism to be signaled when a clickhouse instance is ready to accept queries,
// so we try to send them continuously until the instance accepts them.
// When the instance is not ready, we received the 'Connection refused' exception.
//
// The function accepts stderr for clickhouse-client and determines whether a query has been accepted or not.
func CheckIfClickHouseIsReady(stderr string) bool {
	return !strings.Contains(stderr, "DB::NetException: Connection refused")
}
