package serve

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// The server's own errors, a handler's panic among them, are logged at error
// through its logger, not printed on stderr
func TestHTTPServer_LogsItsErrors(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.InfoLevel)

	server := NewHTTPServer(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic("the handler failed")
		}),
		"127.0.0.1:0",
		zap.New(core),
	)

	ctx, cancel := context.WithCancel(t.Context())
	served := make(chan error, 1)

	go func() {
		served <- server.Serve(ctx)
	}()

	var address string

	require.Eventually(t, func() bool {
		for _, entry := range logs.FilterMessage("HTTP server started").All() {
			address, _ = entry.ContextMap()["address"].(string)
		}

		return address != ""
	}, 5*time.Second, 10*time.Millisecond)

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+address+"/", http.NoBody)
	require.NoError(t, err)

	// The panic closes the connection without an answer
	if response, err := http.DefaultClient.Do(request); err == nil {
		assert.NoError(t, response.Body.Close())
	}

	assert.Eventually(t, func() bool {
		return logs.FilterLevelExact(zap.ErrorLevel).FilterMessageSnippet("the handler failed").Len() == 1
	}, 5*time.Second, 10*time.Millisecond, "the panic was not logged at error")

	cancel()
	require.NoError(t, <-served)
}
