package graph

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A response whose connection could not be taken is still written: the
// upgrader's failure is then answered as any other
func TestHijackAwareWriter_WritesWhenTheConnectionIsNotTaken(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	w := &hijackAwareWriter{ResponseWriter: rec}

	// A recorder, as an HTTP/2 response, cannot be hijacked
	_, _, err := w.Hijack()
	require.ErrorIs(t, err, http.ErrNotSupported)

	w.WriteHeader(http.StatusBadRequest)

	_, err = w.Write([]byte("unable to upgrade"))
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "unable to upgrade", rec.Body.String())
}

// hijackableResponse is a recorded response whose connection can be taken
type hijackableResponse struct {
	*httptest.ResponseRecorder
}

// Hijack takes the connection, which the writer under test never reads
func (hijackableResponse) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, nil
}

// Once the connection is taken, the writer drops the status and fails each
// write with http.ErrHijacked, and the response controller still reaches the
// response it wraps
func TestHijackAwareWriter_DropsWhatIsWrittenOnceTheConnectionIsTaken(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	w := &hijackAwareWriter{ResponseWriter: hijackableResponse{rec}}

	_, _, err := w.Hijack()
	require.NoError(t, err)

	w.WriteHeader(http.StatusBadRequest)

	_, err = w.Write([]byte("unable to upgrade"))
	require.ErrorIs(t, err, http.ErrHijacked)

	// The recorder's own status: none was written to it
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.String())

	require.NoError(t, http.NewResponseController(w).Flush())
	assert.True(t, rec.Flushed, "the controller did not reach the wrapped response")
}

// Only a request carrying an Upgrade header, which gqlgen's WebSocket transport
// takes, reaches next with a hijackAwareWriter: any other reaches it with its
// own response
func TestHijackAware_WrapsTheUpgradeRequestsAlone(t *testing.T) {
	t.Parallel()

	var reached http.ResponseWriter

	handler := hijackAware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = w
	}))

	plain := httptest.NewRecorder()
	handler.ServeHTTP(plain, httptest.NewRequest(http.MethodPost, "/graphql/query", http.NoBody))
	assert.Same(t, plain, reached)

	upgrade := httptest.NewRequest(http.MethodGet, "/graphql/query", http.NoBody)
	upgrade.Header.Set("Upgrade", "websocket")

	handler.ServeHTTP(httptest.NewRecorder(), upgrade)
	assert.IsType(t, &hijackAwareWriter{}, reached)
}

// statusLog is a response that records each status written to it
type statusLog struct {
	*httptest.ResponseRecorder

	statuses []int
}

// WriteHeader records code
func (s *statusLog) WriteHeader(code int) {
	s.statuses = append(s.statuses, code)
}

// The first status written stands: a second, as gqlgen writes after gorilla's
// for an upgrade it refuses, is dropped, as is one after a write, which
// answered 200. An informational status leaves room for the one that follows.
func TestHijackAwareWriter_KeepsTheFirstStatus(t *testing.T) {
	t.Parallel()

	refused := &statusLog{ResponseRecorder: httptest.NewRecorder()}
	w := &hijackAwareWriter{ResponseWriter: refused}

	w.WriteHeader(http.StatusForbidden)
	w.WriteHeader(http.StatusBadRequest)

	_, err := w.Write([]byte("unable to upgrade"))
	require.NoError(t, err)

	assert.Equal(t, []int{http.StatusForbidden}, refused.statuses)
	assert.Equal(t, "unable to upgrade", refused.Body.String())

	written := &statusLog{ResponseRecorder: httptest.NewRecorder()}
	w = &hijackAwareWriter{ResponseWriter: written}

	_, err = w.Write([]byte("an answer"))
	require.NoError(t, err)

	w.WriteHeader(http.StatusBadRequest)

	assert.Empty(t, written.statuses)

	informed := &statusLog{ResponseRecorder: httptest.NewRecorder()}
	w = &hijackAwareWriter{ResponseWriter: informed}

	w.WriteHeader(http.StatusEarlyHints)
	w.WriteHeader(http.StatusForbidden)
	w.WriteHeader(http.StatusBadRequest)

	assert.Equal(t, []int{http.StatusEarlyHints, http.StatusForbidden}, informed.statuses)
}
