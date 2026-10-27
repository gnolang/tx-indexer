package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnolang/tx-indexer/internal/metricstest"
)

// testRequest is a served getBlock call that read 5 rows and answered 1
var testRequest = Request{
	API:          APIJSONRPC,
	Operation:    "getBlock",
	Status:       StatusOK,
	Duration:     2 * time.Second,
	RowsScanned:  5,
	RowsReturned: 1,
}

// A request is in flight from its start to its end, and its end counts it by
// status and records its duration and its rows by operation
func TestMetrics_RecordsARequest(t *testing.T) {
	t.Parallel()

	m := New()

	operation := map[string]string{"api": "jsonrpc", "operation": "getBlock"}

	m.RequestStarted(APIJSONRPC, "getBlock")
	assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_requests_in_flight", operation), 0)

	m.RequestEnded(testRequest)
	assert.InDelta(t, 0, metricstest.Value(t, m, "tx_indexer_requests_in_flight", operation), 0)

	assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_requests_total", map[string]string{
		"api":       "jsonrpc",
		"operation": "getBlock",
		"status":    "ok",
	}), 0)

	duration := metricstest.Find(t, m, "tx_indexer_request_duration_seconds", operation)
	require.NotNil(t, duration)
	assert.Equal(t, uint64(1), duration.GetHistogram().GetSampleCount())
	assert.InDelta(t, 2, duration.GetHistogram().GetSampleSum(), 0)

	assert.InDelta(t, 5, metricstest.Value(t, m, "tx_indexer_rows_scanned_total", operation), 0)
	assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_rows_returned_total", operation), 0)
}

// Each status is counted apart
func TestMetrics_CountsEachStatusApart(t *testing.T) {
	t.Parallel()

	m := New()

	for _, status := range []string{StatusOK, StatusError, StatusDeadline, StatusDeadline} {
		request := testRequest
		request.Status = status

		m.RequestStarted(request.API, request.Operation)
		m.RequestEnded(request)
	}

	for status, want := range map[string]float64{"ok": 1, "error": 1, "deadline": 2} {
		assert.InDelta(t, want, metricstest.Value(t, m, "tx_indexer_requests_total", map[string]string{
			"api":       "jsonrpc",
			"operation": "getBlock",
			"status":    status,
		}), 0, status)
	}
}

// A nil Metrics records nothing, so instrumented code runs without one
func TestMetrics_NilRecordsNothing(t *testing.T) {
	t.Parallel()

	var m *Metrics

	assert.NotPanics(t, func() {
		m.RequestStarted(APIJSONRPC, "getBlock")
		m.RequestEnded(testRequest)
	})
}

// The handler serves every metric in the Prometheus text format, the Go
// runtime's included
func TestMetrics_HandlerServesTheTextFormat(t *testing.T) {
	t.Parallel()

	m := New()

	m.RequestStarted(testRequest.API, testRequest.Operation)
	m.RequestEnded(testRequest)

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))

	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, `tx_indexer_requests_total{api="jsonrpc",operation="getBlock",status="ok"} 1`)
	assert.Contains(t, body, "go_goroutines")
}
