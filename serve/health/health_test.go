package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
