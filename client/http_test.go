package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// newGenesisServer answers every JSON-RPC request with a minimal genesis
// result, echoing the request id, and counts how many times it was hit.
func newGenesisServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)

		var req struct {
			ID json.RawMessage `json:"id"`
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)

			return
		}

		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)

			return
		}

		w.Header().Set("Content-Type", "application/json")

		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  map[string]any{"genesis": nil},
		}

		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)

	return srv, &calls
}

// The chain's genesis is immutable, so the first successful fetch is cached
// and shared: a second call must not hit the wire.
func TestGetGenesisFetchedOnce(t *testing.T) {
	t.Parallel()

	srv, calls := newGenesisServer(t)

	c, err := NewClient(srv.URL)
	require.NoError(t, err)

	first, err := c.GetGenesis(context.Background())
	require.NoError(t, err)

	second, err := c.GetGenesis(context.Background())
	require.NoError(t, err)

	require.Equal(t, first, second)
	require.Equal(t, int32(1), calls.Load(), "the second call must be served from the cache")
}

// The fetcher and the supply bootstrap both ask for the genesis at startup;
// concurrent callers must share one round trip instead of racing for one each.
func TestGetGenesisConcurrentSingleFetch(t *testing.T) {
	t.Parallel()

	srv, calls := newGenesisServer(t)

	c, err := NewClient(srv.URL)
	require.NoError(t, err)

	const callers = 8

	var wg sync.WaitGroup

	first, err := c.GetGenesis(context.Background())
	require.NoError(t, err)
	require.NotNil(t, first)

	wg.Add(callers)

	for range callers {
		go func() {
			defer wg.Done()

			g, err := c.GetGenesis(context.Background())
			if err != nil {
				t.Error(err)

				return
			}

			if g != first {
				t.Error("concurrent callers must see the same cached genesis")
			}
		}()
	}

	wg.Wait()

	require.Equal(t, int32(1), calls.Load(), "concurrent callers must share one fetch")
}
