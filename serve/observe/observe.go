// Package observe records each request the indexer serves, a JSON-RPC call or
// a GraphQL root query field: in the indexer's metrics, and as one line of the
// request log.
package observe

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/gnolang/tx-indexer/internal/tally"
	"github.com/gnolang/tx-indexer/metrics"
	"github.com/gnolang/tx-indexer/serve/deadline"
)

// Observer records the requests both APIs serve. A nil Observer records
// nothing.
type Observer struct {
	metrics *metrics.Metrics

	// log writes the request log; nil leaves it off
	log *zap.Logger

	// clientHeader names the header trusted to carry the client address;
	// empty, the address the request came from is the client's
	clientHeader string

	// trustedHops is the number of trusted proxies appending to clientHeader
	trustedHops int
}

// Option configures an Observer
type Option func(o *Observer)

// WithRequestLog writes one line per request to logger, at info
func WithRequestLog(logger *zap.Logger) Option {
	return func(o *Observer) {
		o.log = logger
	}
}

// WithClientIPHeader trusts header to carry the client address of the request
// log, as a comma-separated list trustedHops proxies append to
func WithClientIPHeader(header string, trustedHops int) Option {
	return func(o *Observer) {
		o.clientHeader = header
		o.trustedHops = trustedHops
	}
}

// New creates an Observer that records into m. A nil m leaves the metrics
// out.
func New(m *metrics.Metrics, opts ...Option) *Observer {
	o := &Observer{
		metrics:     m,
		trustedHops: 1,
	}

	for _, opt := range opts {
		opt(o)
	}

	return o
}

// Middleware carries the client address of each request it passes on in the
// request's context, for the request log. With the request log off, it passes
// requests on as they are.
func (o *Observer) Middleware(next http.Handler) http.Handler {
	if o == nil || o.log == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client := clientAddress(r, o.clientHeader, o.trustedHops)

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientKey{}, client)))
	})
}

// clientKey is the context key the client address is carried under
type clientKey struct{}

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
// whether the query deadline caused it, and carries the client address.
func (c *Call) End(ctx context.Context, failed bool) {
	if c == nil {
		return
	}

	request := metrics.Request{
		API:          c.api,
		Operation:    c.operation,
		Status:       status(ctx, failed),
		Duration:     time.Since(c.started),
		RowsScanned:  c.rows.Scanned(),
		RowsReturned: c.rows.Returned(),
	}

	c.observer.metrics.RequestEnded(request)

	if c.observer.log == nil {
		return
	}

	// The request's arguments are left out: they are the client's to fill,
	// with anything up to the request's size
	client, _ := ctx.Value(clientKey{}).(string)

	c.observer.log.Info(
		"served request",
		zap.String("api", request.API),
		zap.String("operation", request.Operation),
		zap.String("status", request.Status.String()),
		zap.Duration("duration", request.Duration),
		zap.Int64("rows-scanned", request.RowsScanned),
		zap.Int64("rows-returned", request.RowsReturned),
		zap.String("client", client),
	)
}

// status is the outcome of a request served with ctx: a failure is the query
// deadline's once the deadline has ended ctx, whatever error it surfaced as
func status(ctx context.Context, failed bool) metrics.Status {
	if !failed {
		return metrics.StatusOK
	}

	var exceeded *deadline.ExceededError
	if errors.As(context.Cause(ctx), &exceeded) {
		return metrics.StatusDeadline
	}

	return metrics.StatusError
}
