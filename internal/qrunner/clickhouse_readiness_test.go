package qrunner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckIfClickHouseIsReady(t *testing.T) {
	assert.True(t, CheckIfClickHouseIsReady("Returned:\n1 1 2 Helen"))
	assert.False(t, CheckIfClickHouseIsReady("FAILURE: DB::NetException: Connection refused localhost:9000"))
}
