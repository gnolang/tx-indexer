package graph

import (
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
