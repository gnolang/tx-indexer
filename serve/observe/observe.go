// Package observe records each request the indexer serves, a JSON-RPC call or
// a GraphQL root query field, in the indexer's metrics.
package observe

import (
	"context"
	"errors"
	"time"

	"github.com/gnolang/tx-indexer/internal/tally"
	"github.com/gnolang/tx-indexer/metrics"
	"github.com/gnolang/tx-indexer/serve/deadline"
)

// Observer records the requests both APIs serve. A nil Observer records
// nothing.
type Observer struct {
	metrics *metrics.Metrics
}

// New creates an Observer that records into m. A nil m leaves the metrics
// out.
func New(m *metrics.Metrics) *Observer {
	return &Observer{metrics: m}
}

// Call is one request being served
type Call struct {
	observer  *Observer
	started   time.Time
	api       string
	operation string
	rows      tally.Rows
}

// Start starts observing a request of operation over api. The returned
// context carries the call's tally: serve the request with it, so the
// storage and the handlers count the rows it reads and answers. End the call
// once the request is served.
func (o *Observer) Start(ctx context.Context, api, operation string) (context.Context, *Call) {
	if o == nil {
		return ctx, nil
	}

	call := &Call{
		observer:  o,
		started:   time.Now(),
		api:       api,
		operation: operation,
	}

	o.metrics.RequestStarted(api, operation)

	return tally.NewContext(ctx, &call.rows), call
}

// End records the call once its request is served. failed tells whether the
// request was answered with an error; ctx, the request's context, tells
// whether the query deadline caused it.
func (c *Call) End(ctx context.Context, failed bool) {
	if c == nil {
		return
	}

	c.observer.metrics.RequestEnded(metrics.Request{
		API:          c.api,
		Operation:    c.operation,
		Status:       status(ctx, failed),
		Duration:     time.Since(c.started),
		RowsScanned:  c.rows.Scanned(),
		RowsReturned: c.rows.Returned(),
	})
}

// status is the outcome of a request served with ctx: a failure is the query
// deadline's once the deadline has ended ctx, whatever error it surfaced as
func status(ctx context.Context, failed bool) string {
	if !failed {
		return metrics.StatusOK
	}

	var exceeded *deadline.ExceededError
	if errors.As(context.Cause(ctx), &exceeded) {
		return metrics.StatusDeadline
	}

	return metrics.StatusError
}
