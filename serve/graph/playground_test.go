package graph

import (
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

// goneClient is a response whose every write fails, as one to a client that
// reset its connection does
type goneClient struct {
	*httptest.ResponseRecorder
}

// Write fails with EPIPE
func (goneClient) Write([]byte) (int, error) {
	return 0, syscall.EPIPE
}

// The playground is served as an HTML page holding its title, its endpoint and
// its default tabs
func TestHandlerWithDefaultTabs_ServesThePlayground(t *testing.T) {
	t.Parallel()

	handler := HandlerWithDefaultTabs("Indexer playground", "/graphql/query", []string{"{ latestBlockHeight }"})

	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodGet, "/graphql", http.NoBody))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/html; charset=UTF-8", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Body.String(), "<title>Indexer playground</title>")
	assert.Contains(t, rec.Body.String(), "/graphql/query")
	assert.Contains(t, rec.Body.String(), "latestBlockHeight")
}

// A client gone while the playground is written is no failure of the
// indexer's: the handler returns, where a panic would have the HTTP server log
// it at error with its stack
func TestHandlerWithDefaultTabs_LeavesAGoneClient(t *testing.T) {
	t.Parallel()

	handler := HandlerWithDefaultTabs("Indexer playground", "/graphql/query", []string{"{ latestBlockHeight }"})

	assert.NotPanics(t, func() {
		handler(goneClient{httptest.NewRecorder()}, httptest.NewRequest(http.MethodGet, "/graphql", http.NoBody))
	})
}
