package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/internal/metricstest"
	"github.com/gnolang/tx-indexer/internal/tally"
	"github.com/gnolang/tx-indexer/metrics"
	"github.com/gnolang/tx-indexer/serve/metadata"
	"github.com/gnolang/tx-indexer/serve/observe"
	"github.com/gnolang/tx-indexer/serve/spec"
)

// requestsOf are the labels counting the calls of method with status
func requestsOf(method, status string) map[string]string {
	return map[string]string{"api": "jsonrpc", "operation": method, "status": status}
}

// rowsCall reads 5 rows and answers 2, counting them in its request's tally
func rowsCall(ctx context.Context, _ *metadata.Metadata, _ []any) (any, *spec.BaseJSONError) {
	tally.FromContext(ctx).AddScanned(5)
	tally.FromContext(ctx).AddReturned(2)

	return "rows", nil
}

// failingCall answers an error that is not the query deadline's
func failingCall(context.Context, *metadata.Metadata, []any) (any, *spec.BaseJSONError) {
	return nil, spec.GenerateResponseError(errors.New("storage unavailable"))
}

// Each call is counted by its method and status, with its duration and the
// rows it read and answered
func TestJSONRPC_RecordsEachCall(t *testing.T) {
	t.Parallel()

	m := metrics.New()

	j := NewJSONRPC(nil, WithObserver(observe.New(m)))
	j.RegisterHandler("rows", rowsCall)
	j.RegisterHandler("failing", failingCall)

	serveRequest(t, j, spec.BaseJSONRequests{
		spec.NewJSONRequest(1, "rows", nil),
		spec.NewJSONRequest(2, "rows", nil),
		spec.NewJSONRequest(3, "failing", nil),
	})

	assert.InDelta(t, 2, metricstest.Value(t, m, "tx_indexer_requests_total", requestsOf("rows", "ok")), 0)
	assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_requests_total", requestsOf("failing", "error")), 0)

	rowsLabels := map[string]string{"api": "jsonrpc", "operation": "rows"}

	assert.InDelta(t, 2, metricstest.Value(t, m, "tx_indexer_request_duration_seconds", rowsLabels), 0)
	assert.InDelta(t, 10, metricstest.Value(t, m, "tx_indexer_rows_scanned_total", rowsLabels), 0)
	assert.InDelta(t, 4, metricstest.Value(t, m, "tx_indexer_rows_returned_total", rowsLabels), 0)
	assert.InDelta(t, 0, metricstest.Value(t, m, "tx_indexer_requests_in_flight", rowsLabels), 0)
}

// A call is in flight while its handler runs
func TestJSONRPC_CountsACallInFlight(t *testing.T) {
	t.Parallel()

	m := metrics.New()

	var inFlight float64

	j := NewJSONRPC(nil, WithObserver(observe.New(m)))
	j.RegisterHandler(
		"probe",
		func(context.Context, *metadata.Metadata, []any) (any, *spec.BaseJSONError) {
			inFlight = metricstest.Value(t, m, "tx_indexer_requests_in_flight", map[string]string{
				"api":       "jsonrpc",
				"operation": "probe",
			})

			return nil, nil
		},
	)

	serveRequest(t, j, spec.NewJSONRequest(1, "probe", nil))

	assert.InDelta(t, 1, inFlight, 0)
}

// A call whose handler panics leaves nothing in flight, and counts as error:
// the panic goes on up, as it does without an observer
func TestJSONRPC_CountsAPanickingCallAsFailed(t *testing.T) {
	t.Parallel()

	m := metrics.New()

	j := NewJSONRPC(nil, WithObserver(observe.New(m)))
	j.RegisterHandler(
		"panicking",
		func(context.Context, *metadata.Metadata, []any) (any, *spec.BaseJSONError) {
			panic("storage corrupted")
		},
	)

	assert.Panics(t, func() {
		serveRequest(t, j, spec.NewJSONRequest(1, "panicking", nil))
	})

	panickingLabels := map[string]string{"api": "jsonrpc", "operation": "panicking"}

	assert.InDelta(t, 0, metricstest.Value(t, m, "tx_indexer_requests_in_flight", panickingLabels), 0)
	assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_requests_total", requestsOf("panicking", "error")), 0)
}

// A call the query deadline stops counts as deadline, and so does each call of
// its batch the deadline answers without running
func TestJSONRPC_CountsADeadlineHit(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		m := metrics.New()

		j := NewJSONRPC(nil, WithQueryDeadline(testDeadline), WithObserver(observe.New(m)))
		j.RegisterHandler("slow", slowCall)
		j.RegisterHandler("rows", rowsCall)

		serveRequest(t, j, spec.BaseJSONRequests{
			spec.NewJSONRequest(1, "slow", nil),
			spec.NewJSONRequest(2, "rows", nil),
		})

		assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_requests_total", requestsOf("slow", "deadline")), 0)
		assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_requests_total", requestsOf("rows", "deadline")), 0)

		slowLabels := map[string]string{"api": "jsonrpc", "operation": "slow"}

		duration := metricstest.Find(t, m, "tx_indexer_request_duration_seconds", slowLabels)
		require.NotNil(t, duration)
		assert.InDelta(t, testDeadline.Seconds(), duration.GetHistogram().GetSampleSum(), 0)
	})
}

// A call naming no registered method runs no operation and is not counted:
// its method, which the client picks, never becomes a label
func TestJSONRPC_LeavesAnUnknownMethodUncounted(t *testing.T) {
	t.Parallel()

	m := metrics.New()

	j := NewJSONRPC(nil, WithObserver(observe.New(m)))

	response := decodeResponse[spec.BaseJSONResponse](
		t,
		serveRequest(t, j, spec.NewJSONRequest(1, "made-up", nil)),
	)
	require.NotNil(t, response.Error)
	assert.Equal(t, spec.MethodNotFoundErrorCode, response.Error.Code)

	for _, status := range []string{"ok", "error", "deadline"} {
		assert.Nil(t, metricstest.Find(t, m, "tx_indexer_requests_total", requestsOf("made-up", status)))
	}
}

// paramMarker is an argument value a log line must never carry
const paramMarker = "a-value-only-the-client-chose"

// assertNoMarker asserts no line of logs carries paramMarker
func assertNoMarker(t *testing.T, logs *observer.ObservedLogs) {
	t.Helper()

	for _, entry := range logs.All() {
		assert.NotContains(t, entry.Message, paramMarker)

		for key, value := range entry.ContextMap() {
			assert.NotContains(t, fmt.Sprint(value), paramMarker, key)
		}
	}
}

// loggingObserver logs each call to the returned logs, trusting one proxy to
// append the client address to X-Forwarded-For
func loggingObserver() (*observe.Observer, *observer.ObservedLogs) {
	core, logs := observer.New(zap.InfoLevel)

	return observe.New(nil, observe.WithRequestLog(zap.New(core)), observe.WithClientIPHeader("X-Forwarded-For", 1)), logs
}

// Each call over HTTP is logged with the client address its trusted header
// names, and never with its params
func TestJSONRPC_LogsEachCall(t *testing.T) {
	t.Parallel()

	o, logs := loggingObserver()

	j := NewJSONRPC(nil, WithObserver(o))
	j.RegisterHandler("rows", rowsCall)

	body, err := json.Marshal(spec.NewJSONRequest(1, "rows", []any{paramMarker}))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", jsonMimeType)
	req.Header.Set("X-Forwarded-For", "6.6.6.6, 203.0.113.9")

	rec := httptest.NewRecorder()
	j.SetupRoutes(chi.NewMux()).ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	entries := logs.All()
	require.Len(t, entries, 1)

	fields := entries[0].ContextMap()
	assert.Equal(t, "rows", fields["operation"])
	assert.Equal(t, "203.0.113.9", fields["client"])
	assert.Equal(t, int64(5), fields["rows-scanned"])

	assertNoMarker(t, logs)
}

// A call sent over the WS endpoint is logged with the client address the
// trusted header of the connection's upgrade request names
func TestJSONRPC_LogsEachCallOverWS(t *testing.T) {
	t.Parallel()

	o, logs := loggingObserver()

	j := NewJSONRPC(events.NewManager(), WithObserver(o))
	j.RegisterHandler("rows", rowsCall)

	server := httptest.NewServer(j.SetupRoutes(chi.NewMux()))
	defer server.Close()

	conn, resp, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"/ws",
		http.Header{"X-Forwarded-For": {"203.0.113.9"}},
	)
	require.NoError(t, err)

	defer func() {
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, conn.Close())
	}()

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	require.NoError(t, conn.WriteJSON(spec.NewJSONRequest(1, "rows", []any{paramMarker})))

	var response spec.BaseJSONResponse

	require.NoError(t, conn.ReadJSON(&response))
	require.Nil(t, response.Error)

	var served []observer.LoggedEntry

	for _, entry := range logs.All() {
		if entry.Message == "served request" {
			served = append(served, entry)
		}
	}

	require.Len(t, served, 1)
	assert.Equal(t, "203.0.113.9", served[0].ContextMap()["client"])

	assertNoMarker(t, logs)
}
