package serve

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/serve/budget"
	"github.com/gnolang/tx-indexer/serve/metadata"
	"github.com/gnolang/tx-indexer/serve/spec"
)

// A call over a WebSocket whose handler panics still releases its client's
// reservation, so the client is admitted again
func TestJSONRPC_ReleasesAPanickingCallOverWS(t *testing.T) {
	t.Parallel()

	// Room for one request in flight
	b := budget.New(30*time.Second, 30*time.Second, 30*time.Second)
	charged := budget.Client{Budget: b, Key: "203.0.113.9", Address: "203.0.113.9"}

	reached := make(chan struct{})

	j := NewJSONRPC(events.NewManager())
	j.RegisterHandler("panics", func(context.Context, *metadata.Metadata, []any) (any, *spec.BaseJSONError) {
		close(reached)
		panic("the handler failed")
	})

	mux := j.SetupRoutes(chi.NewMux())

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(budget.NewContext(r.Context(), charged)))
	}))

	// The server recovers the panic, and logs it with its stack
	server.Config.ErrorLog = log.New(io.Discard, "", 0)

	server.Start()
	defer server.Close()

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	require.NoError(t, err)

	defer func() {
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, conn.Close())
	}()

	require.NoError(t, conn.WriteJSON(spec.NewJSONRequest(1, "panics", nil)))

	// The call holds the one place in flight until its panic unwinds
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "the call never reached its handler")
	}

	assert.Eventually(t, func() bool {
		end, err := b.Reserve(charged.Key)
		if err != nil {
			return false
		}

		end()

		return true
	}, 5*time.Second, 10*time.Millisecond, "the client's reservation was never released")
}
