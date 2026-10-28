package main

import (
	"context"
	"net/http"
	"testing"
	"time"

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

// TestStartCfg_TimeBudgetFlags makes sure the time budget is off unless the
// operator sets it.
func TestStartCfg_TimeBudgetFlags(t *testing.T) {
	t.Parallel()

	cfg := testCfg(t)

	assert.Zero(t, cfg.clientTimeBudget)
	assert.Zero(t, cfg.clientTimeBudgetRefill)

	cfg = testCfg(t, "--client-time-budget", "60s", "--client-time-budget-refill", "30s")

	assert.Equal(t, time.Minute, cfg.clientTimeBudget)
	assert.Equal(t, 30*time.Second, cfg.clientTimeBudgetRefill)
}

// A time budget set inconsistently fails at startup, naming the flags at
// fault: one of its two flags without the other, a negative value, a refill
// of 0, which would leave a client that spent its budget refused for good,
// and a budget without the query deadline that bounds each request's
// overdraw
func TestStartCfg_InconsistentTimeBudgetRefusesToStart(t *testing.T) {
	t.Parallel()

	both := []string{"--client-time-budget", "--client-time-budget-refill"}

	testTable := []struct {
		name  string
		args  []string
		names []string
	}{
		{"the budget alone", []string{"--client-time-budget", "60s", "--query-deadline", "30s"}, both},
		{"the refill alone", []string{"--client-time-budget-refill", "30s", "--query-deadline", "30s"}, both},
		{"the budget off, the refill alone", []string{"--client-time-budget-refill", "30s"}, both},
		{
			"a negative budget",
			[]string{"--client-time-budget", "-60s", "--client-time-budget-refill", "30s", "--query-deadline", "30s"},
			both,
		},
		{
			"a negative refill",
			[]string{"--client-time-budget", "60s", "--client-time-budget-refill", "-30s", "--query-deadline", "30s"},
			both,
		},
		{
			"a refill of 0",
			[]string{"--client-time-budget", "60s", "--client-time-budget-refill", "0s", "--query-deadline", "30s"},
			both,
		},
		{
			"no query deadline",
			[]string{"--client-time-budget", "60s", "--client-time-budget-refill", "30s"},
			[]string{"--client-time-budget", "--query-deadline"},
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			err := testCfg(t, testCase.args...).exec(ctx)
			require.Error(t, err)

			for _, name := range testCase.names {
				assert.ErrorContains(t, err, name)
			}
		})
	}
}

// A time budget set with both its flags, a refill and a query deadline
// serves, and so does one turned off with both its flags
func TestStartCfg_ConsistentTimeBudgetServes(t *testing.T) {
	t.Parallel()

	startIndexer(t, false, "--client-time-budget", "60s", "--client-time-budget-refill", "30s", "--query-deadline", "30s")
	startIndexer(t, false, "--client-time-budget", "0s", "--client-time-budget-refill", "0s")
}
