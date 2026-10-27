package metricstest

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Find matches a sample on all its labels and their values, never on a part
// of them
func TestFind_MatchesLabelsExactly(t *testing.T) {
	t.Parallel()

	registry := prometheus.NewRegistry()

	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "test_total", Help: "A test counter."},
		[]string{"api", "status"},
	)
	require.NoError(t, registry.Register(counter))

	counter.WithLabelValues("jsonrpc", "ok").Add(2)
	counter.WithLabelValues("jsonrpc", "error").Add(3)

	assert.InDelta(t, 2, Value(t, registry, "test_total", map[string]string{"api": "jsonrpc", "status": "ok"}), 0)
	assert.InDelta(t, 3, Value(t, registry, "test_total", map[string]string{"api": "jsonrpc", "status": "error"}), 0)

	assert.Nil(t, Find(t, registry, "test_total", map[string]string{"api": "jsonrpc"}))
	assert.Nil(t, Find(t, registry, "test_total", map[string]string{"api": "graphql", "status": "ok"}))
	assert.Nil(t, Find(t, registry, "other_total", map[string]string{"api": "jsonrpc", "status": "ok"}))
}
