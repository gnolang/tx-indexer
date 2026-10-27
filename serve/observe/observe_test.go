package observe

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
