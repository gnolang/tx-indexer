package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The client limits apply to the API routes alone: /health and /ready, which
// a load balancer's health checks read, and the playground are never refused
func TestStartCfg_LimitsLeaveTheOtherRoutesAlone(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, false, "--http-rate-limit", "1")

	for _, path := range []string{"/health", "/ready", "/graphql"} {
		for range 3 {
			response := get(t, "http://"+started.apiAddress+path)
			require.NotNil(t, response, path)
			assert.NotEqual(t, http.StatusTooManyRequests, response.status, path)
		}
	}
}
