package ws

import (
	"testing"

	"github.com/olahol/melody"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// A response written to a session that is not open, as one its client
// closed, is logged at warn: a client alone causes it, and error marks the
// indexer's own failure
func TestResponseWriter_LogsAClosedSessionAtWarn(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.DebugLevel)

	New(zap.New(core), &melody.Session{}).WriteResponse("an answer")

	unwritten := logs.FilterMessage("unable to write WS response").All()
	require.Len(t, unwritten, 1)

	assert.Equal(t, zap.WarnLevel, unwritten[0].Level)
	assert.Equal(t, melody.ErrSessionClosed.Error(), unwritten[0].ContextMap()["error"])
}

// A response that does not encode is the indexer's own failure, logged at
// error, and nothing is written
func TestResponseWriter_LogsAnUnencodableResponseAtError(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.DebugLevel)

	New(zap.New(core), &melody.Session{}).WriteResponse(make(chan int))

	unencoded := logs.FilterMessage("unable to encode JSON-RPC response").All()
	require.Len(t, unencoded, 1)

	assert.Equal(t, zap.ErrorLevel, unencoded[0].Level)
	assert.Empty(t, logs.FilterMessage("unable to write WS response").All())
}
