package graph

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/99designs/gqlgen/client"
	"github.com/99designs/gqlgen/graphql"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/internal/metricstest"
	"github.com/gnolang/tx-indexer/metrics"
	"github.com/gnolang/tx-indexer/serve/observe"
	"github.com/gnolang/tx-indexer/storage"
	indexerTypes "github.com/gnolang/tx-indexer/types"
)

// fieldNamed is a root field of name, as gqlgen collects it
func fieldNamed(name string) *ast.Field {
	return &ast.Field{Name: name, Alias: name}
}

// fieldLabels are the labels of the GraphQL requests of field
func fieldLabels(field string) map[string]string {
	return map[string]string{"api": "graphql", "operation": field}
}

// fieldRequests are the labels counting the GraphQL requests of field with
// status
func fieldRequests(field, status string) map[string]string {
	return map[string]string{"api": "graphql", "operation": field, "status": status}
}

// observedStore returns a store holding 10 heights of one block and three
// transactions each, served by a mux that records into the returned metrics
func observedStore(t *testing.T) (*metrics.Metrics, *chi.Mux) {
	t.Helper()

	r := testResolver(t)
	seedBlocks(t, r, 1, 10)
	seedTxs(t, r, 10, 3)

	m := metrics.New()

	return m, Setup(r.store, events.NewManager(), nil, chi.NewMux(), false, 0, observe.New(m))
}

// Each root field of a query is one request, named after the field and not
// its alias, with its duration and the rows it read and answered: a scan
// counts every key it walks, the ones its index window passes over included
func TestObserve_RecordsEachRootField(t *testing.T) {
	t.Parallel()

	m, mux := observedStore(t)

	resp := postQuery(t, mux, `{
		windowed: getTransactions(where: {index: {gt: 1}}) { block_height }
		getBlocks(where: {}) { height }
	}`)
	require.Empty(t, resp.Errors)

	for field, rows := range map[string]struct{ scanned, returned float64 }{
		"getTransactions": {30, 10},
		"getBlocks":       {10, 10},
	} {
		labels := fieldLabels(field)

		assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_requests_total", fieldRequests(field, "ok")), 0, field)
		assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_request_duration_seconds", labels), 0, field)
		assert.InDelta(t, rows.scanned, metricstest.Value(t, m, "tx_indexer_rows_scanned_total", labels), 0, field)
		assert.InDelta(t, rows.returned, metricstest.Value(t, m, "tx_indexer_rows_returned_total", labels), 0, field)
		assert.InDelta(t, 0, metricstest.Value(t, m, "tx_indexer_requests_in_flight", labels), 0, field)
	}

	assert.Nil(t, metricstest.Find(t, m, "tx_indexer_requests_total", fieldRequests("windowed", "ok")))
}

// A transaction looked up by its hash is one row read and answered
func TestObserve_CountsAHashLookupAsOneRow(t *testing.T) {
	t.Parallel()

	hash := base64.StdEncoding.EncodeToString(types.Tx{1, 0}.Hash())

	for field, query := range map[string]string{
		"transactions":    fmt.Sprintf(`{ transactions(filter: {hash: %q}) { block_height } }`, hash),
		"getTransactions": fmt.Sprintf(`{ getTransactions(where: {hash: {eq: %q}}) { block_height } }`, hash),
	} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			m, mux := observedStore(t)

			resp := postQuery(t, mux, query)
			require.Empty(t, resp.Errors)

			assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_rows_scanned_total", fieldLabels(field)), 0)
			assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_rows_returned_total", fieldLabels(field)), 0)
		})
	}
}

// A list query the query deadline stops counts as deadline
func TestObserve_CountsADeadlineHit(t *testing.T) {
	t.Parallel()

	for field, query := range listQueries {
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				m := metrics.New()
				mux := Setup(
					&slowStore{rows: slowRows},
					events.NewManager(),
					nil,
					chi.NewMux(),
					false,
					testDeadline,
					observe.New(m),
				)

				resp := postQuery(t, mux, query)
				require.Len(t, resp.Errors, 1)

				assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_requests_total", fieldRequests(field, "deadline")), 0)
			})
		})
	}
}

// failingStore fails to read the latest height
type failingStore struct {
	storage.Storage
}

func (failingStore) GetLatestHeight() (uint64, error) {
	return 0, errors.New("storage unavailable")
}

// slowFailingStore reads the latest height past the query deadline, taking
// no context, then fails
type slowFailingStore struct {
	storage.Storage
}

func (slowFailingStore) GetLatestHeight() (uint64, error) {
	time.Sleep(testDeadline + 10*time.Millisecond)

	return 0, errors.New("pebble: corrupted sstable")
}

// A root field that fails on its own error after the query deadline has
// passed is answered that error, and counts as error: only a field answered
// the deadline counts as deadline
func TestObserve_CountsAnOwnErrorPastTheDeadlineAsError(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		m := metrics.New()
		mux := Setup(slowFailingStore{}, events.NewManager(), nil, chi.NewMux(), false, testDeadline, observe.New(m))

		resp := postQuery(t, mux, `{ latestBlockHeight }`)
		require.Len(t, resp.Errors, 1)
		assert.Equal(t, "pebble: corrupted sstable", resp.Errors[0].Message)

		failed := fieldRequests("latestBlockHeight", "error")
		assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_requests_total", failed), 0)
		assert.Nil(t, metricstest.Find(t, m, "tx_indexer_requests_total", fieldRequests("latestBlockHeight", "deadline")))
	})
}

// A root field whose resolver answers an error counts as error
func TestObserve_CountsAFailedField(t *testing.T) {
	t.Parallel()

	m := metrics.New()
	mux := Setup(failingStore{}, events.NewManager(), nil, chi.NewMux(), false, 0, observe.New(m))

	resp := postQuery(t, mux, `{ latestBlockHeight }`)
	require.Len(t, resp.Errors, 1)

	failed := fieldRequests("latestBlockHeight", "error")
	assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_requests_total", failed), 0)
}

// A root field whose argument does not coerce fails before its resolver runs,
// and counts as error like a JSON-RPC call with an invalid param
func TestObserve_CountsAnArgumentThatDoesNotCoerceAsFailed(t *testing.T) {
	t.Parallel()

	r := testResolver(t)
	seedBlocks(t, r, 1, 3)

	m := metrics.New()
	mux := Setup(r.store, events.NewManager(), nil, chi.NewMux(), false, 0, observe.New(m))

	resp := postQuery(t, mux, `{ blocks(filter: {from_time: "not-a-time"}) { height } }`)
	require.Len(t, resp.Errors, 1)

	assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_requests_total", fieldRequests("blocks", "error")), 0)
	assert.Nil(t, metricstest.Find(t, m, "tx_indexer_requests_total", fieldRequests("blocks", "ok")))
}

// A resolver that panics never returns its error, and gqlgen recovers the
// panic below the root field: the field still counts as error. The extension
// is driven the way gqlgen drives it, so the recovered panic prints no stack.
func TestObserve_CountsAPanickingResolverAsFailed(t *testing.T) {
	t.Parallel()

	m := metrics.New()
	extension := observeExtension{observer: observe.New(m)}

	ctx := graphql.WithRootFieldContext(context.Background(), &graphql.RootFieldContext{
		Object: "latestBlockHeight",
		Field:  graphql.CollectedField{Field: fieldNamed("latestBlockHeight")},
	})

	extension.InterceptRootField(ctx, func(ctx context.Context) graphql.Marshaler {
		fieldCtx := graphql.WithFieldContext(ctx, &graphql.FieldContext{
			Object: "Query",
			Field:  graphql.CollectedField{Field: fieldNamed("latestBlockHeight")},
		})

		func() {
			defer func() {
				assert.NotNil(t, recover())
			}()

			res, err := extension.InterceptField(fieldCtx, func(context.Context) (any, error) {
				panic("storage corrupted")
			})

			assert.Fail(t, "the resolver returned", "%v, %v", res, err)
		}()

		return graphql.Null
	})

	failed := fieldRequests("latestBlockHeight", "error")
	assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_requests_total", failed), 0)
}

// contextProbeStore notes, when its block scan runs, whether the scan's
// context carries the answer the observe extension notes for its root field
type contextProbeStore struct {
	storage.Storage
	scanned  *atomic.Bool
	observed *atomic.Bool
}

func (s contextProbeStore) BlockIterator(uint64, uint64) (storage.Iterator[*types.Block], error) {
	return &contextProbeIterator{store: s}, nil
}

// contextProbeIterator is the empty scan of a contextProbeStore
type contextProbeIterator struct {
	store contextProbeStore
}

func (it *contextProbeIterator) Next(ctx context.Context) bool {
	it.store.scanned.Store(true)
	it.store.observed.Store(ctx.Value(rootAnswerKey{}) != nil)

	return false
}

func (it *contextProbeIterator) Value() (*types.Block, error) {
	return nil, nil
}

func (it *contextProbeIterator) Error() error {
	return nil
}

func (it *contextProbeIterator) Close() error {
	return nil
}

// With no observer, the observe extension is not installed: a query's fields
// run with nothing of it, and cost what they cost without it
func TestObserve_NoObserverInstallsNothing(t *testing.T) {
	t.Parallel()

	store := contextProbeStore{scanned: &atomic.Bool{}, observed: &atomic.Bool{}}
	mux := Setup(store, events.NewManager(), nil, chi.NewMux(), false, 0, nil)

	resp := postQuery(t, mux, `{ getBlocks(where: {}) { height } }`)
	require.Empty(t, resp.Errors)

	require.True(t, store.scanned.Load(), "the scan never ran")
	assert.False(t, store.observed.Load(), "the observe extension ran with no observer")
}

// blockingStore holds its block scan until released, so a test can look at
// the request while it is being served
type blockingStore struct {
	storage.Storage
	started chan struct{}
	release chan struct{}
}

func (s blockingStore) BlockIterator(uint64, uint64) (storage.Iterator[*types.Block], error) {
	close(s.started)
	<-s.release

	return &slowIterator[*types.Block]{value: slowBlock}, nil
}

// A root field is in flight while its resolver runs
func TestObserve_CountsAFieldInFlight(t *testing.T) {
	t.Parallel()

	m := metrics.New()
	store := blockingStore{started: make(chan struct{}), release: make(chan struct{})}
	mux := Setup(store, events.NewManager(), nil, chi.NewMux(), false, 0, observe.New(m))

	req := httptest.NewRequest(
		http.MethodPost,
		"/graphql/query",
		strings.NewReader(`{"query": "{ getBlocks(where: {}) { height } }"}`),
	)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	served := make(chan struct{})

	go func() {
		defer close(served)

		mux.ServeHTTP(rec, req)
	}()

	<-store.started

	// Read while the scan is held, asserted once it is released, so a failure
	// leaves no request blocked
	inFlight := metricstest.Find(t, m, "tx_indexer_requests_in_flight", fieldLabels("getBlocks"))

	close(store.release)
	<-served

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NotNil(t, inFlight, "no getBlocks request in flight")
	assert.InDelta(t, 1, inFlight.GetGauge().GetValue(), 0)
	assert.InDelta(t, 0, metricstest.Value(t, m, "tx_indexer_requests_in_flight", fieldLabels("getBlocks")), 0)
}

// A subscription lasts as long as its connection: it is not a request, and
// is not counted
func TestObserve_LeavesSubscriptionsUncounted(t *testing.T) {
	t.Parallel()

	m := metrics.New()
	manager := events.NewManager()
	mux := Setup(&slowStore{}, manager, nil, chi.NewMux(), false, 0, observe.New(m))

	sub := client.New(mux, client.Path("/graphql/query")).
		Websocket(`subscription { getBlocks(where: {}) { height } }`)

	defer func() {
		assert.NoError(t, sub.Close())
	}()

	// The subscription registers with the event manager asynchronously, so
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
				manager.SignalEvent(&indexerTypes.NewBlock{Block: slowBlock(height)})
			}
		}
	}()

	var resp struct {
		GetBlocks struct {
			Height int64 `json:"height"`
		} `json:"getBlocks"`
	}

	require.NoError(t, sub.Next(&resp))

	for _, status := range []string{"ok", "error", "deadline"} {
		assert.Nil(t, metricstest.Find(t, m, "tx_indexer_requests_total", fieldRequests("getBlocks", status)))
	}
}

// Each root field is logged with the client address its trusted header
// names, and never with the query's variables
func TestObserve_LogsEachRootField(t *testing.T) {
	t.Parallel()

	const variableMarker = "a-value-only-the-client-chose"

	core, logs := observer.New(zap.InfoLevel)
	o := observe.New(nil, observe.WithRequestLog(zap.New(core)), observe.WithClientIPHeader("X-Forwarded-For", 1))

	r := testResolver(t)
	mux := Setup(r.store, events.NewManager(), nil, chi.NewMux(), false, 0, o)

	body, err := json.Marshal(map[string]any{
		"query":     `query($hash: String!) { getTransactions(where: {hash: {eq: $hash}}) { block_height } }`,
		"variables": map[string]any{"hash": variableMarker},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/graphql/query", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "6.6.6.6, 203.0.113.9")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	entries := logs.All()
	require.Len(t, entries, 1)

	fields := entries[0].ContextMap()
	assert.Equal(t, "graphql", fields["api"])
	assert.Equal(t, "getTransactions", fields["operation"])
	assert.Equal(t, "203.0.113.9", fields["client"])

	assert.NotContains(t, entries[0].Message, variableMarker)

	for key, value := range fields {
		assert.NotContains(t, fmt.Sprint(value), variableMarker, key)
	}
}

// A query sent over the WebSocket transport is logged with the client address
// the trusted header of the connection's upgrade request names
func TestObserve_LogsAFieldOverWebSocket(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.InfoLevel)
	o := observe.New(nil, observe.WithRequestLog(zap.New(core)), observe.WithClientIPHeader("X-Forwarded-For", 1))

	r := testResolver(t)
	mux := Setup(r.store, events.NewManager(), nil, chi.NewMux(), false, 0, o)

	var resp map[string]any

	require.NoError(t, client.New(mux, client.Path("/graphql/query")).WebsocketOnce(
		`{ getBlocks(where: {}) { height } }`,
		&resp,
		client.AddHeader("X-Forwarded-For", "203.0.113.9"),
	))

	entries := logs.All()
	require.Len(t, entries, 1)

	assert.Equal(t, "getBlocks", entries[0].ContextMap()["operation"])
	assert.Equal(t, "203.0.113.9", entries[0].ContextMap()["client"])
}
