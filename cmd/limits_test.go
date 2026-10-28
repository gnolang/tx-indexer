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

// refusedSeries is the series of the refusals over api for reason, as the
// scrape writes it
func refusedSeries(api, reason string) string {
	return `tx_indexer_requests_refused_total{api="` + api + `",reason="` + reason + `"}`
}

// A request past the count limit is counted as refused for the rate limit,
// under the API its path serves, and never as served
func TestStartCfg_CountsARateLimitRefusal(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, true, "--http-rate-limit", "1")

	assert.Equal(
		t,
		[]int{http.StatusOK, http.StatusTooManyRequests},
		statuses(t, started.apiAddress, http.Header{}, http.Header{}),
	)

	graphql := fetchURL(
		t,
		http.MethodPost,
		"http://"+started.apiAddress+"/graphql/query",
		http.Header{"Content-Type": {"application/json"}},
		`{"query": "{ getBlocks(where: {}) { height } }"}`,
	)
	require.NotNil(t, graphql)
	assert.Equal(t, http.StatusTooManyRequests, graphql.status)

	scrape := get(t, "http://"+started.metricsAddress+"/metrics")
	require.NotNil(t, scrape)

	for series, want := range map[string]string{
		refusedSeries("jsonrpc", "http_rate_limit"):    "1",
		refusedSeries("graphql", "http_rate_limit"):    "1",
		refusedSeries("jsonrpc", "client_time_budget"): "0",
		refusedSeries("graphql", "client_time_budget"): "0",
	} {
		assert.Equal(t, series+" "+want, sample(scrape.body, series))
	}

	served := `tx_indexer_requests_total{api="jsonrpc",operation="getBlock",status="ok"}`
	assert.Equal(t, served+" 1", sample(scrape.body, served))
}
