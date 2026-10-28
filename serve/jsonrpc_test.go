package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnolang/tx-indexer/events"
)

// A call that is null is answered as an invalid request, with no ID, beside
// the other calls of its batch, rather than panicking the handler
func TestJSONRPC_AnswersANullCallAsInvalid(t *testing.T) {
	t.Parallel()

	invalid := map[string]any{
		"jsonrpc": "2.0",
		"result":  nil,
		"error":   map[string]any{"code": float64(-32600), "message": "invalid JSON-RPC 2.0 request"},
	}

	testTable := []struct {
		want any
		name string
		body string
	}{
		{invalid, "a batch of null", `[null]`},
		{[]any{invalid, invalid}, "a batch of two nulls", `[null, null]`},
	}

	mux := NewJSONRPC(events.NewManager()).SetupRoutes(chi.NewMux())

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(testCase.body))
			req.Header.Set("Content-Type", "application/json")

			rec := httptest.NewRecorder()

			require.NotPanics(t, func() {
				mux.ServeHTTP(rec, req)
			})

			var answer any

			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &answer), rec.Body.String())
			assert.Equal(t, testCase.want, answer)
		})
	}
}
