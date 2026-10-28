package serve

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// A request to /ws that does not upgrade, which any client may send, is
// answered 400 and logged at warn: error marks the indexer's own failure
func TestJSONRPC_LogsAFailedWSUpgradeAtWarn(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.DebugLevel)

	j := NewJSONRPC(nil, WithLogger(zap.New(core)))

	rec := httptest.NewRecorder()
	j.SetupRoutes(chi.NewMux()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ws", nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	failed := logs.FilterMessage("unable to initialize WS connection").All()
	require.Len(t, failed, 1)

	assert.Equal(t, zap.WarnLevel, failed[0].Level)
	assert.Contains(t, failed[0].ContextMap()["error"], "websocket")
}
