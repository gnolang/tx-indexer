package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnolang/tx-indexer/storage"
)

// countingChecker is a node that is ready, counting the checks it answers
type countingChecker struct {
	checks atomic.Int64
}

// IsReady answers ready, and counts the check
func (c *countingChecker) IsReady(context.Context) (bool, error) {
	c.checks.Add(1)

	return true, nil
}

// requireReady requires mux to answer /ready with 200
func requireReady(t *testing.T, mux http.Handler) {
	t.Helper()

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", http.NoBody))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "node is ready")
}

// /ready answers from a check of the node made at most once a second: each
// check costs the node a status RPC, and no client limit refuses /ready, so
// 200 requests within a second cost one, and the next second's another
func TestReady_ChecksTheNodeOnceASecond(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		checker := &countingChecker{}
		mux := Setup(nil, checker, chi.NewMux())

		for range 200 {
			requireReady(t, mux)
		}

		assert.Equal(t, int64(1), checker.checks.Load())

		time.Sleep(time.Second)
		requireReady(t, mux)

		assert.Equal(t, int64(2), checker.checks.Load())
	})
}

// contextChecker is a node that is ready, but that a check whose context has
// ended cannot reach
type contextChecker struct{}

// IsReady answers ctx's error, if ctx has ended, and ready otherwise
func (contextChecker) IsReady(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	return true, nil
}

// A request that leaves while the node is checked does not end the check:
// its answer, which serves every request for a second, is the node's, so no
// client can have the node answer not ready by leaving
func TestReady_ChecksTheNodeBeyondTheRequest(t *testing.T) {
	t.Parallel()

	mux := Setup(nil, contextChecker{}, chi.NewMux())

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", http.NoBody).WithContext(ctx))

	requireReady(t, mux)
}

// notReadyChecker is a node that is not ready
type notReadyChecker struct{}

// IsReady answers not ready, with the reason
func (notReadyChecker) IsReady(context.Context) (bool, error) {
	return false, errors.New("node down")
}

// heightStorage is a store that answers its latest height, or its error. It
// holds the Storage interface for the methods /health never calls.
type heightStorage struct {
	storage.Storage

	err    error
	height uint64
}

// GetLatestHeight answers the store's height, or its error
func (s heightStorage) GetLatestHeight() (uint64, error) {
	return s.height, s.err
}

// get answers mux's response to a GET of path
func get(mux http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, http.NoBody))

	return rec
}

// /ready answers 500 when the node is not ready, so a load balancer or a
// probe reading the status sees it, and 200 when it is
func TestReady_AnswersTheNodesState(t *testing.T) {
	t.Parallel()

	notReady := get(Setup(nil, notReadyChecker{}, chi.NewMux()), "/ready")

	assert.Equal(t, http.StatusInternalServerError, notReady.Code)
	assert.Contains(t, notReady.Body.String(), "node not ready: node down")

	ready := get(Setup(nil, &countingChecker{}, chi.NewMux()), "/ready")

	assert.Equal(t, http.StatusOK, ready.Code)
	assert.Contains(t, ready.Body.String(), "node is ready")
}

// /health answers 500 when the store cannot answer its height, and 200 with
// the height when it can
func TestHealth_AnswersTheStoresState(t *testing.T) {
	t.Parallel()

	failing := get(Setup(heightStorage{err: errors.New("closed")}, nil, chi.NewMux()), "/health")

	assert.Equal(t, http.StatusInternalServerError, failing.Code)
	assert.Contains(t, failing.Body.String(), "storage is not reachable: closed")

	healthy := get(Setup(heightStorage{height: 42}, nil, chi.NewMux()), "/health")

	assert.Equal(t, http.StatusOK, healthy.Code)
	assert.Contains(t, healthy.Body.String(), `"height":42`)
}
