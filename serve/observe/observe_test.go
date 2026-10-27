package observe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/gnolang/tx-indexer/internal/metricstest"
	"github.com/gnolang/tx-indexer/internal/tally"
	"github.com/gnolang/tx-indexer/metrics"
	"github.com/gnolang/tx-indexer/serve/deadline"
)

// operationLabels are the labels of the getBlock calls the tests serve
var operationLabels = map[string]string{"api": "jsonrpc", "operation": "getBlock"}

// requestsWithStatus are the labels counting the getBlock calls with status
func requestsWithStatus(status string) map[string]string {
	return map[string]string{"api": "jsonrpc", "operation": "getBlock", "status": status}
}

// A call is in flight from its start to its end, and its end records it with
// its duration and the rows counted in the tally its context carries
func TestObserver_RecordsACall(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		m := metrics.New()
		o := New(m)

		ctx, call := o.Start(context.Background(), metrics.APIJSONRPC, "getBlock")

		assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_requests_in_flight", operationLabels), 0)

		tally.FromContext(ctx).AddScanned(7)
		tally.FromContext(ctx).AddReturned(2)

		time.Sleep(3 * time.Second)

		call.End(ctx, false)

		assert.InDelta(t, 0, metricstest.Value(t, m, "tx_indexer_requests_in_flight", operationLabels), 0)
		assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_requests_total", requestsWithStatus("ok")), 0)
		assert.InDelta(t, 7, metricstest.Value(t, m, "tx_indexer_rows_scanned_total", operationLabels), 0)
		assert.InDelta(t, 2, metricstest.Value(t, m, "tx_indexer_rows_returned_total", operationLabels), 0)

		duration := metricstest.Find(t, m, "tx_indexer_request_duration_seconds", operationLabels)
		require.NotNil(t, duration)
		assert.InDelta(t, 3, duration.GetHistogram().GetSampleSum(), 0)
	})
}

// A call's status tells a failure the query deadline caused from any other
func TestObserver_ClassifiesTheStatus(t *testing.T) {
	t.Parallel()

	// pastDeadline is a context the query deadline has ended
	pastDeadline := func() context.Context {
		ctx, cancel := deadline.WithTimeout(context.Background(), time.Nanosecond)
		t.Cleanup(cancel)

		<-ctx.Done()

		return ctx
	}

	// clientLeft is a context its client has cancelled
	clientLeft := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		return ctx
	}

	testTable := []struct {
		ctx    func() context.Context
		name   string
		status string
		failed bool
	}{
		{context.Background, "answered", "ok", false},
		{context.Background, "failed", "error", true},
		{pastDeadline, "failed past the query deadline", "deadline", true},
		{pastDeadline, "answered as the query deadline passed", "ok", false},
		{clientLeft, "failed as its client left", "error", true},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			m := metrics.New()

			ctx, call := New(m).Start(testCase.ctx(), metrics.APIJSONRPC, "getBlock")
			call.End(ctx, testCase.failed)

			assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_requests_total", requestsWithStatus(testCase.status)), 0)
		})
	}
}

// A nil Observer observes nothing: its context carries no tally, and its call
// ends without recording
func TestObserver_NilObservesNothing(t *testing.T) {
	t.Parallel()

	var o *Observer

	ctx, call := o.Start(context.Background(), metrics.APIJSONRPC, "getBlock")

	assert.Nil(t, tally.FromContext(ctx))
	assert.NotPanics(t, func() {
		call.End(ctx, false)
	})
}

// observedLog is a logger whose entries the test reads
func observedLog() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zap.InfoLevel)

	return zap.New(core), logs
}

// serveThrough sends a request from peer, carrying header, through o's
// middleware to serve
func serveThrough(o *Observer, peer string, header http.Header, serve http.HandlerFunc) {
	r := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	r.RemoteAddr = peer
	r.Header = header

	o.Middleware(serve).ServeHTTP(httptest.NewRecorder(), r)
}

// With the request log on, each call ends with one line at info: its API,
// operation, status, duration and rows, and the client address the
// middleware read from the trusted header
func TestObserver_LogsEachCall(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logger, logs := observedLog()
		o := New(nil, WithRequestLog(logger), WithClientIPHeader("X-Forwarded-For", 1))

		header := http.Header{"X-Forwarded-For": {"6.6.6.6, 203.0.113.9"}}

		serveThrough(o, "10.0.0.7:41234", header, func(_ http.ResponseWriter, r *http.Request) {
			ctx, call := o.Start(r.Context(), metrics.APIJSONRPC, "getBlock")

			tally.FromContext(ctx).AddScanned(4)
			tally.FromContext(ctx).AddReturned(1)

			time.Sleep(2 * time.Second)

			call.End(ctx, true)
		})

		entries := logs.All()
		require.Len(t, entries, 1)

		assert.Equal(t, zap.InfoLevel, entries[0].Level)
		assert.Equal(t, "served request", entries[0].Message)
		assert.Equal(t, map[string]any{
			"api":           "jsonrpc",
			"operation":     "getBlock",
			"status":        "error",
			"duration":      2 * time.Second,
			"rows-scanned":  int64(4),
			"rows-returned": int64(1),
			"client":        "203.0.113.9",
		}, entries[0].ContextMap())
	})
}

// With no header trusted, the client address is the address the request came
// from
func TestObserver_LogsThePeerWithNoHeaderTrusted(t *testing.T) {
	t.Parallel()

	logger, logs := observedLog()
	o := New(nil, WithRequestLog(logger))

	header := http.Header{"X-Forwarded-For": {"203.0.113.9"}}

	serveThrough(o, "10.0.0.7:41234", header, func(_ http.ResponseWriter, r *http.Request) {
		ctx, call := o.Start(r.Context(), metrics.APIGraphQL, "getBlocks")
		call.End(ctx, false)
	})

	entries := logs.All()
	require.Len(t, entries, 1)
	assert.Equal(t, "10.0.0.7", entries[0].ContextMap()["client"])
}

// A nil Observer's middleware passes requests on as they are
func TestObserver_NilMiddlewarePassesRequestsOn(t *testing.T) {
	t.Parallel()

	var (
		o      *Observer
		served bool
	)

	serveThrough(o, "10.0.0.7:41234", http.Header{}, func(http.ResponseWriter, *http.Request) {
		served = true
	})

	assert.True(t, served)
}
