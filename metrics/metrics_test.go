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

	for _, status := range []Status{StatusOK, StatusError, StatusDeadline, StatusDeadline} {
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
		m.SetIndexedHeight(1)
		m.SetChainHeight(1)
		m.CountFetchError(FetchStageRange)
		m.CountFetchRetry()
		m.RequestRefused(APIJSONRPC, RefusalClientTimeBudget)
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

// The fetcher's heights and failures are recorded as it reports them. The
// fetch errors are exported from the start, so an alert on their rate has a
// series to read before the first error.
func TestMetrics_RecordsTheFetch(t *testing.T) {
	t.Parallel()

	m := New()

	chainHeightErrors := map[string]string{"stage": "chain_height"}
	rangeErrors := map[string]string{"stage": "range"}

	assert.InDelta(t, 0, metricstest.Value(t, m, "tx_indexer_fetch_errors_total", chainHeightErrors), 0)
	assert.InDelta(t, 0, metricstest.Value(t, m, "tx_indexer_fetch_errors_total", rangeErrors), 0)

	m.SetIndexedHeight(7)
	m.SetChainHeight(9)
	m.CountFetchError(FetchStageRange)
	m.CountFetchRetry()

	assert.InDelta(t, 7, metricstest.Value(t, m, "tx_indexer_indexed_height", nil), 0)
	assert.InDelta(t, 9, metricstest.Value(t, m, "tx_indexer_chain_height", nil), 0)
	assert.InDelta(t, 0, metricstest.Value(t, m, "tx_indexer_fetch_errors_total", chainHeightErrors), 0)
	assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_fetch_errors_total", rangeErrors), 0)
	assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_fetch_retries_total", nil), 0)

	m.CountFetchError(FetchStageChainHeight)
	assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_fetch_errors_total", chainHeightErrors), 0)
}

// A refused request is counted by API and reason alone, every pair starting at
// zero, and never among the requests served
func TestMetrics_CountsARefusal(t *testing.T) {
	t.Parallel()

	m := New()

	for _, api := range []string{APIGraphQL, APIJSONRPC} {
		for _, reason := range []string{"client_time_budget", "http_rate_limit"} {
			refused := map[string]string{"api": api, "reason": reason}
			assert.InDelta(t, 0, metricstest.Value(t, m, "tx_indexer_requests_refused_total", refused), 0, refused)
		}
	}

	m.RequestRefused(APIJSONRPC, RefusalClientTimeBudget)
	m.RequestRefused(APIJSONRPC, RefusalClientTimeBudget)
	m.RequestRefused(APIGraphQL, RefusalHTTPRateLimit)

	for refused, want := range map[[2]string]float64{
		{APIJSONRPC, "client_time_budget"}: 2,
		{APIJSONRPC, "http_rate_limit"}:    0,
		{APIGraphQL, "client_time_budget"}: 0,
		{APIGraphQL, "http_rate_limit"}:    1,
	} {
		labels := map[string]string{"api": refused[0], "reason": refused[1]}
		assert.InDelta(t, want, metricstest.Value(t, m, "tx_indexer_requests_refused_total", labels), 0, labels)
	}

	assert.Nil(t, metricstest.Find(t, m, "tx_indexer_requests_total", map[string]string{
		"api":       "jsonrpc",
		"operation": "",
		"status":    "refused",
	}))
}

// Each refusal reason is the reason label's value
func TestRefusal_String(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "client_time_budget", RefusalClientTimeBudget.String())
	assert.Equal(t, "http_rate_limit", RefusalHTTPRateLimit.String())
	assert.Equal(t, "Refusal(9)", Refusal(9).String())
}
