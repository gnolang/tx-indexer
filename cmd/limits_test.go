package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

// The client limits apply to the API routes alone: /health and /ready, which
// a load balancer's health checks read, and the playground are never refused
func TestStartCfg_LimitsLeaveTheOtherRoutesAlone(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, false, append([]string{"--http-rate-limit", "1"}, spendingBudget...)...)

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

	// The upgrade to JSON-RPC's WebSocket is the API's too
	_, upgrade, err := websocket.DefaultDialer.DialContext(t.Context(), "ws://"+started.apiAddress+"/ws", nil)
	require.ErrorIs(t, err, websocket.ErrBadHandshake)
	assert.Equal(t, http.StatusTooManyRequests, upgrade.StatusCode)
	assert.NoError(t, upgrade.Body.Close())

	scrape := get(t, "http://"+started.metricsAddress+"/metrics")
	require.NotNil(t, scrape)

	for series, want := range map[string]string{
		refusedSeries("jsonrpc", "http_rate_limit"):    "2",
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

// spendingBudget are the flags of a time budget the first request of any
// client spends: 1 ns of indexer time, regained at 1 ns a minute
var spendingBudget = []string{
	"--client-time-budget", "1ns",
	"--client-time-budget-refill", "1ns",
	"--query-deadline", "10s",
}

// postTo sends body to path at address, as JSON, and returns the answer
func postTo(t *testing.T, address, path, body string) *httpResponse {
	t.Helper()

	header := http.Header{"Content-Type": {"application/json"}}

	response := fetchURL(t, http.MethodPost, "http://"+address+path, header, body)
	require.NotNil(t, response, "nothing answers at %s%s", address, path)

	return response
}

// requireRetryAfter is the Retry-After of a refusal, in whole seconds, which
// must be at least one
func requireRetryAfter(t *testing.T, refused *httpResponse) int {
	t.Helper()

	require.Equal(t, http.StatusTooManyRequests, refused.status, refused.body)

	seconds, err := strconv.Atoi(refused.header.Get("Retry-After"))
	require.NoError(t, err)
	require.GreaterOrEqual(t, seconds, 1)

	return seconds
}

// A JSON-RPC request of a client that spent its budget is answered 429, with
// Retry-After, and an error with code -32005 in place of each call's result,
// under each call's ID: a batch as a batch, and a body that is no JSON-RPC
// request with one error and no ID
func TestStartCfg_TimeBudgetRefusesJSONRPCCalls(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, false, spendingBudget...)

	assert.Equal(t, http.StatusOK, postTo(t, started.apiAddress, "/", jsonrpcCall).status)

	// The refusal of call id names the Retry-After of the response it is in
	refusal := func(id, seconds int) map[string]any {
		answer := map[string]any{
			"jsonrpc": "2.0",
			"result":  nil,
			"error": map[string]any{
				"code":    float64(-32005),
				"message": "client time budget exhausted; retry in " + (time.Duration(seconds) * time.Second).String(),
				"data": map[string]any{
					"code":              "CLIENT_TIME_BUDGET_EXHAUSTED",
					"retryAfterSeconds": float64(seconds),
				},
			},
		}

		if id != 0 {
			answer["id"] = float64(id)
		}

		return answer
	}

	var answer any

	single := postTo(t, started.apiAddress, "/", `{"jsonrpc": "2.0", "id": 7, "method": "getBlock", "params": ["1"]}`)
	seconds := requireRetryAfter(t, single)

	require.NoError(t, json.Unmarshal([]byte(single.body), &answer))
	assert.Equal(t, refusal(7, seconds), answer)

	batch := postTo(t, started.apiAddress, "/", `[
		{"jsonrpc": "2.0", "id": 8, "method": "getBlock", "params": ["1"]},
		{"jsonrpc": "2.0", "id": 9, "method": "getBlock", "params": ["2"]}
	]`)
	seconds = requireRetryAfter(t, batch)

	require.NoError(t, json.Unmarshal([]byte(batch.body), &answer))
	assert.Equal(t, []any{refusal(8, seconds), refusal(9, seconds)}, answer)

	nonsense := postTo(t, started.apiAddress, "/", `nonsense`)
	seconds = requireRetryAfter(t, nonsense)

	require.NoError(t, json.Unmarshal([]byte(nonsense.body), &answer))
	assert.Equal(t, refusal(0, seconds), answer)
}

// A GraphQL request of a client that spent its budget is answered 429, with
// Retry-After, and a GraphQL error in the shape of the query deadline's
func TestStartCfg_TimeBudgetRefusesGraphQLQueries(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, false, spendingBudget...)

	query := `{"query": "{ getBlocks(where: {}) { height } }"}`

	assert.Equal(t, http.StatusOK, postTo(t, started.apiAddress, "/graphql/query", query).status)

	refused := postTo(t, started.apiAddress, "/graphql/query", query)
	seconds := requireRetryAfter(t, refused)

	var answer any

	require.NoError(t, json.Unmarshal([]byte(refused.body), &answer))
	assert.Equal(t, map[string]any{
		"data": nil,
		"errors": []any{map[string]any{
			"message": "client time budget exhausted; retry in " + (time.Duration(seconds) * time.Second).String(),
			"extensions": map[string]any{
				"code":              "CLIENT_TIME_BUDGET_EXHAUSTED",
				"retryAfterSeconds": float64(seconds),
			},
		}},
	}, answer)
}

// Two clients behind the trusted proxy are charged apart, each under the
// address the proxy names
func TestStartCfg_TimeBudgetChargesClientsApart(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, false, append(spendingBudget, "--client-ip-header", "CF-Connecting-IP")...)

	assert.Equal(
		t,
		[]int{http.StatusOK, http.StatusTooManyRequests, http.StatusOK},
		statuses(
			t,
			started.apiAddress,
			http.Header{"Cf-Connecting-Ip": {"203.0.113.9"}},
			http.Header{"Cf-Connecting-Ip": {"203.0.113.9"}},
			http.Header{"Cf-Connecting-Ip": {"203.0.113.10"}},
		),
	)
}

// A client behind two trusted proxies is charged under the address the first
// appended to X-Forwarded-For: what it writes itself never makes it a new
// client, and a request that claims an upgrade it cannot make is charged too
func TestStartCfg_TimeBudgetIgnoresForgedAddresses(t *testing.T) {
	t.Parallel()

	started := startIndexer(
		t,
		false,
		append(spendingBudget, "--client-ip-header", "X-Forwarded-For", "--client-ip-trusted-hops", "2")...,
	)

	assert.Equal(
		t,
		[]int{http.StatusOK, http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests},
		statuses(
			t,
			started.apiAddress,
			http.Header{"X-Forwarded-For": {"6.6.6.1, 203.0.113.9, 198.51.100.4"}},
			http.Header{"X-Forwarded-For": {"203.0.113.9, 198.51.100.5"}, "X-Real-Ip": {"6.6.6.2"}},
			http.Header{"X-Forwarded-For": {"203.0.113.9, 198.51.100.6"}, "True-Client-Ip": {"6.6.6.3"}},
			http.Header{"X-Forwarded-For": {"203.0.113.9, 198.51.100.6"}, "Upgrade": {"websocket"}},
		),
	)
}

// heldRequest is a JSON-RPC request whose body the test holds back, so the
// indexer serves it until the test releases it
type heldRequest struct {
	body     *io.PipeWriter
	answered chan *httpResponse
}

// holdRequest sends the JSON-RPC endpoint at address a request whose body it
// holds back, and returns once the indexer reads the body: the request is
// then admitted, and in flight until released
func holdRequest(t *testing.T, address string) *heldRequest {
	t.Helper()

	reader, writer := io.Pipe()

	t.Cleanup(func() {
		assert.NoError(t, writer.CloseWithError(errors.New("the test ended")))
	})

	reading := make(chan struct{})
	ctx := httptrace.WithClientTrace(t.Context(), &httptrace.ClientTrace{
		Got100Continue: func() {
			close(reading)
		},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+address+"/", reader)
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")

	// The body goes once the indexer reads it, which it answers 100 Continue
	req.Header.Set("Expect", "100-continue")

	transport := &http.Transport{ExpectContinueTimeout: time.Minute}
	t.Cleanup(transport.CloseIdleConnections)

	held := &heldRequest{body: writer, answered: make(chan *httpResponse, 1)}

	go func() {
		resp, err := (&http.Client{Transport: transport}).Do(req)
		if err != nil {
			held.answered <- nil

			return
		}

		read, err := io.ReadAll(resp.Body)

		held.answered <- &httpResponse{
			header: resp.Header,
			body:   string(read),
			status: resp.StatusCode,
		}

		assert.NoError(t, errors.Join(err, resp.Body.Close()))
	}()

	select {
	case <-reading:
		return held
	case answer := <-held.answered:
		require.FailNow(t, "the request was answered before its body was read", "%+v", answer)
	case <-time.After(10 * time.Second):
		require.FailNow(t, "the indexer never read the request's body")
	}

	return nil
}

// release sends the held body, and returns the answer
func (h *heldRequest) release(t *testing.T) *httpResponse {
	t.Helper()

	_, err := io.WriteString(h.body, jsonrpcCall)
	require.NoError(t, err)
	require.NoError(t, h.body.Close())

	select {
	case answer := <-h.answered:
		require.NotNil(t, answer, "the request failed")

		return answer
	case <-time.After(10 * time.Second):
		require.FailNow(t, "the released request was never answered")

		return nil
	}
}

// A client whose slow requests spent its budget is refused, with Retry-After,
// and served again once that many seconds have refilled it. A request is
// charged its wall time, its body's upload included.
func TestStartCfg_TimeBudgetRefillsASlowClient(t *testing.T) {
	t.Parallel()

	// 200 ms of indexer time, regained at 200 ms a second
	started := startIndexer(
		t,
		false,
		"--client-time-budget", "200ms",
		"--client-time-budget-refill", "12s",
		"--query-deadline", "10s",
	)

	held := holdRequest(t, started.apiAddress)

	time.Sleep(600 * time.Millisecond)
	assert.Equal(t, http.StatusOK, held.release(t).status)

	seconds := requireRetryAfter(t, postTo(t, started.apiAddress, "/", jsonrpcCall))

	time.Sleep(time.Duration(seconds) * time.Second)
	assert.Equal(t, http.StatusOK, postTo(t, started.apiAddress, "/", jsonrpcCall).status)
}

// Each request in flight holds one query deadline of its client's budget, so a
// client overdraws one deadline at most: with a 60 s budget and a 30 s
// deadline, a third request is refused while two run
func TestStartCfg_TimeBudgetReservesTheDeadlineInFlight(t *testing.T) {
	t.Parallel()

	started := startIndexer(
		t,
		false,
		"--client-time-budget", "60s",
		"--client-time-budget-refill", "30s",
		"--query-deadline", "30s",
	)

	first := holdRequest(t, started.apiAddress)
	second := holdRequest(t, started.apiAddress)

	assert.Equal(t, 1, requireRetryAfter(t, postTo(t, started.apiAddress, "/", jsonrpcCall)))

	assert.Equal(t, http.StatusOK, first.release(t).status)
	assert.Equal(t, http.StatusOK, second.release(t).status)
	assert.Equal(t, http.StatusOK, postTo(t, started.apiAddress, "/", jsonrpcCall).status)
}

// A request the time budget refuses is counted as refused for the budget,
// under the API its path serves
func TestStartCfg_CountsATimeBudgetRefusal(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, true, spendingBudget...)

	query := `{"query": "{ getBlocks(where: {}) { height } }"}`

	for range 2 {
		postTo(t, started.apiAddress, "/", jsonrpcCall)
	}

	for range 3 {
		postTo(t, started.apiAddress, "/graphql/query", query)
	}

	scrape := get(t, "http://"+started.metricsAddress+"/metrics")
	require.NotNil(t, scrape)

	// The first request served spent the budget: every later one is refused
	for series, want := range map[string]string{
		refusedSeries("jsonrpc", "client_time_budget"): "1",
		refusedSeries("graphql", "client_time_budget"): "3",
		refusedSeries("jsonrpc", "http_rate_limit"):    "0",
		refusedSeries("graphql", "http_rate_limit"):    "0",
	} {
		assert.Equal(t, series+" "+want, sample(scrape.body, series))
	}
}

// TestStartCfg_GraphQLTokenLimitFlag makes sure a GraphQL document is held to
// 4000 tokens unless the operator sets another limit, 0 for none.
func TestStartCfg_GraphQLTokenLimitFlag(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 4000, testCfg(t).graphqlTokenLimit)
	assert.Equal(t, 0, testCfg(t, "--graphql-token-limit", "0").graphqlTokenLimit)
}

// A negative token limit fails at startup, naming the flag
func TestStartCfg_NegativeGraphQLTokenLimitRefusesToStart(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.ErrorContains(t, testCfg(t, "--graphql-token-limit", "-1").exec(ctx), "--graphql-token-limit")
}

// A GraphQL document over the token limit is refused before it is validated,
// over HTTP; one under it is served
func TestStartCfg_RefusesAGraphQLDocumentOverTheTokenLimit(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, false, "--graphql-token-limit", "10")

	// 3 tokens, then 12
	served := postTo(t, started.apiAddress, "/graphql/query", `{"query": "{ __typename }"}`)
	assert.Equal(t, http.StatusOK, served.status, served.body)
	assert.NotContains(t, served.body, "errors")

	refused := postTo(t, started.apiAddress, "/graphql/query", `{"query": "{ getBlocks(where: {}) { height } }"}`)
	assert.Equal(t, http.StatusUnprocessableEntity, refused.status)
	assert.Contains(t, refused.body, "exceeded token limit of 10")
}

// Every example query of the playground parses under the default token limit
func TestStartCfg_GraphQLTokenLimitHoldsTheExamples(t *testing.T) {
	t.Parallel()

	examples, err := filepath.Glob("../serve/graph/examples/*.gql")
	require.NoError(t, err)
	require.NotEmpty(t, examples)

	for _, example := range examples {
		query, err := os.ReadFile(example)
		require.NoError(t, err)

		_, err = parser.ParseQueryWithTokenLimit(&ast.Source{Input: string(query)}, defaultGraphQLTokenLimit)
		assert.NoError(t, err, example)
	}
}
