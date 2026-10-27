package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	bftTypes "github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/serve/metadata"
	"github.com/gnolang/tx-indexer/serve/spec"
	"github.com/gnolang/tx-indexer/types"
)

const (
	// testDeadline is the query deadline the tests configure
	testDeadline = 50 * time.Millisecond

	// testDeadlineMessage is the error a call answers once testDeadline passes
	testDeadlineMessage = "query deadline exceeded (50ms); narrow the block height range"
)

// testDeadlineData is the machine-readable part of that error
var testDeadlineData = map[string]any{
	"code":          "QUERY_DEADLINE_EXCEEDED",
	"queryDeadline": "50ms",
}

// callTaking is a method that runs for d, or until its request's context
// ends if that comes first: a scan that honours its context
func callTaking(d time.Duration) Handler {
	return func(ctx context.Context, _ *metadata.Metadata, _ []any) (any, *spec.BaseJSONError) {
		select {
		case <-ctx.Done():
			return nil, spec.GenerateResponseError(context.Cause(ctx))
		case <-time.After(d):
			return "finished", nil
		}
	}
}

// slowCall runs for far longer than testDeadline: a scan with no bound of
// its own
var slowCall = callTaking(100 * testDeadline)

// serveRequest sends request to the HTTP endpoint of j in-process, so a test
// can run it inside a synctest bubble, and returns the body
func serveRequest(t *testing.T, j *JSONRPC, request any) []byte {
	t.Helper()

	body, err := json.Marshal(request)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", jsonMimeType)

	rec := httptest.NewRecorder()
	j.SetupRoutes(chi.NewMux()).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	return rec.Body.Bytes()
}

// A call that runs past the deadline is stopped at the deadline, not before,
// and answers an error naming it
func TestJSONRPC_QueryDeadlineStopsASlowCall(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		j := NewJSONRPC(nil, WithQueryDeadline(testDeadline))
		j.RegisterHandler("slow", slowCall)

		start := time.Now()
		raw := serveRequest(t, j, spec.NewJSONRequest(1, "slow", nil))

		assert.Equal(t, testDeadline, time.Since(start), "the call was not stopped at the deadline")

		response := decodeResponse[spec.BaseJSONResponse](t, raw)

		assert.Nil(t, response.Result)
		require.NotNil(t, response.Error)
		assert.Equal(t, spec.ServerErrorCode, response.Error.Code)
		assert.Equal(t, testDeadlineMessage, response.Error.Message)
		assert.Equal(t, testDeadlineData, response.Error.Data)
	})
}

// A call that ends inside the deadline answers in full
func TestJSONRPC_QueryDeadlineLeavesACallInsideItWhole(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const took = testDeadline - 10*time.Millisecond

		j := NewJSONRPC(nil, WithQueryDeadline(testDeadline))
		j.RegisterHandler("quick", callTaking(took))

		start := time.Now()
		response := decodeResponse[spec.BaseJSONResponse](
			t,
			serveRequest(t, j, spec.NewJSONRequest(1, "quick", nil)),
		)

		assert.Equal(t, took, time.Since(start))
		assert.Nil(t, response.Error)
		assert.Equal(t, "finished", response.Result)
	})
}

// dialWS opens a WS connection to the JSON-RPC endpoint of server, with a read
// deadline so a missing answer fails the test instead of hanging it
func dialWS(t *testing.T, server *httptest.Server) *websocket.Conn {
	t.Helper()

	conn, resp, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"/ws",
		nil,
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, conn.Close())
	})

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))

	return conn
}

// A call sent over the WS endpoint is bounded like one sent over HTTP: the
// deadline follows the call, not the transport
func TestJSONRPC_QueryDeadlineStopsASlowCallOverWS(t *testing.T) {
	t.Parallel()

	j := NewJSONRPC(events.NewManager(), WithQueryDeadline(testDeadline))
	j.RegisterHandler("slow", slowCall)

	server := httptest.NewServer(j.SetupRoutes(chi.NewMux()))
	defer server.Close()

	conn := dialWS(t, server)
	require.NoError(t, conn.WriteJSON(spec.NewJSONRequest(1, "slow", nil)))

	var response spec.BaseJSONResponse

	require.NoError(t, conn.ReadJSON(&response))
	require.NotNil(t, response.Error)
	assert.Equal(t, testDeadlineMessage, response.Error.Message)
	assert.Equal(t, testDeadlineData, response.Error.Data)
}

// A deadline of zero is the default, and leaves every call unbounded
func TestJSONRPC_ZeroLeavesCallsUnbounded(t *testing.T) {
	t.Parallel()

	j := NewJSONRPC(nil, WithQueryDeadline(0))
	j.RegisterHandler(
		"bounded",
		func(ctx context.Context, _ *metadata.Metadata, _ []any) (any, *spec.BaseJSONError) {
			_, bounded := ctx.Deadline()

			return bounded, nil
		},
	)

	response := decodeResponse[spec.BaseJSONResponse](
		t,
		serveRequest(t, j, spec.NewJSONRequest(1, "bounded", nil)),
	)

	assert.Nil(t, response.Error)
	assert.Equal(t, false, response.Result)
}

// The calls of a batch share one deadline: once a call has used it up, the
// calls after it answer the deadline without running
func TestJSONRPC_QueryDeadlineCoversABatch(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var probed atomic.Bool

		j := NewJSONRPC(nil, WithQueryDeadline(testDeadline))
		j.RegisterHandler("slow", slowCall)
		j.RegisterHandler(
			"probe",
			func(context.Context, *metadata.Metadata, []any) (any, *spec.BaseJSONError) {
				probed.Store(true)

				return "ran", nil
			},
		)

		start := time.Now()
		responses := decodeResponse[spec.BaseJSONResponses](
			t,
			serveRequest(t, j, spec.BaseJSONRequests{
				spec.NewJSONRequest(1, "slow", nil),
				spec.NewJSONRequest(2, "probe", nil),
			}),
		)

		assert.Equal(t, testDeadline, time.Since(start), "the batch was not stopped at the deadline")
		require.Len(t, *responses, 2)

		for _, response := range *responses {
			require.NotNil(t, response.Error, "call %d", response.ID)
			assert.Equal(t, testDeadlineMessage, response.Error.Message, "call %d", response.ID)
			assert.Equal(t, testDeadlineData, response.Error.Data, "call %d", response.ID)
		}

		assert.False(t, probed.Load(), "a call ran after the batch's deadline")
	})
}

// A subscription lives as long as its WS connection, so the deadline must not
// end it: one older than the deadline still delivers the next block
func TestJSONRPC_SubscriptionOutlivesTheDeadline(t *testing.T) {
	t.Parallel()

	manager := events.NewManager()

	j := NewJSONRPC(manager, WithQueryDeadline(testDeadline))
	j.RegisterSubEndpoints(nil)

	server := httptest.NewServer(j.SetupRoutes(chi.NewMux()))
	defer server.Close()

	conn := dialWS(t, server)
	require.NoError(t, conn.WriteJSON(spec.NewJSONRequest(1, "subscribe", []any{"newHeads"})))

	var subscribed spec.BaseJSONResponse

	require.NoError(t, conn.ReadJSON(&subscribed))
	require.Nil(t, subscribed.Error)

	time.Sleep(3 * testDeadline)

	// The filter manager registers with the event manager asynchronously, so
	// keep announcing blocks until one arrives
	stop := make(chan struct{})
	defer close(stop)

	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		for height := int64(1); ; height++ {
			select {
			case <-stop:
				return
			case <-ticker.C:
				manager.SignalEvent(&types.NewBlock{
					Block: &bftTypes.Block{Header: bftTypes.Header{Height: height}},
				})
			}
		}
	}()

	var event spec.BaseJSONSubscribeResponse

	require.NoError(t, conn.ReadJSON(&event))
	assert.Equal(t, spec.SubscriptionMethod, event.Method)
	require.NotNil(t, event.Params)
	assert.Equal(t, subscribed.Result, event.Params.Subscription)
}
