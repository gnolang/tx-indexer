package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnolang/tx-indexer/serve/spec"
)

// dialWS opens a WebSocket connection to url in subprotocol, if any, and
// closes it when the test ends
func dialWS(t *testing.T, url string, subprotocols ...string) *websocket.Conn {
	t.Helper()

	dialer := websocket.Dialer{Subprotocols: subprotocols, HandshakeTimeout: 5 * time.Second}

	conn, resp, err := dialer.DialContext(t.Context(), url, nil)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, resp.Body.Close())
		assert.NoError(t, conn.Close())
	})

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(10*time.Second)))

	return conn
}

// callWS sends one JSON-RPC call over conn and returns its answer
func callWS(t *testing.T, conn *websocket.Conn, id uint, method string, params ...any) spec.BaseJSONResponse {
	t.Helper()

	require.NoError(t, conn.WriteJSON(spec.NewJSONRequest(id, method, params)))

	var response spec.BaseJSONResponse

	require.NoError(t, conn.ReadJSON(&response))
	require.Equal(t, id, response.ID)

	return response
}

// requireBudgetRefusal requires err to be the time budget's refusal, its
// Retry-After at least one second
func requireBudgetRefusal(t *testing.T, details map[string]any) {
	t.Helper()

	assert.Equal(t, "CLIENT_TIME_BUDGET_EXHAUSTED", details["code"])

	seconds, ok := details["retryAfterSeconds"].(float64)
	require.True(t, ok, "no retryAfterSeconds in %v", details)
	assert.GreaterOrEqual(t, seconds, float64(1))
}

// Each call sent over JSON-RPC's WebSocket is charged to the client that
// opened the connection, and refused with the time budget's error once that
// client has spent it. A subscription is neither charged nor refused, and the
// upgrade itself is never refused.
func TestStartCfg_TimeBudgetChargesEachCallOverJSONRPCWebSocket(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, false, spendingBudget...)
	url := "ws://" + started.apiAddress + "/ws"

	conn := dialWS(t, url)

	// Not charged: the budget is whole for the call after it
	subscribed := callWS(t, conn, 1, "subscribe", "newHeads")
	require.Nil(t, subscribed.Error)

	served := callWS(t, conn, 2, "getBlock", "1")
	if served.Error != nil {
		require.NotEqual(t, spec.ClientTimeBudgetErrorCode, served.Error.Code)
	}

	refused := callWS(t, conn, 3, "getBlock", "1")
	require.NotNil(t, refused.Error)
	assert.Equal(t, spec.ClientTimeBudgetErrorCode, refused.Error.Code)

	details, ok := refused.Error.Data.(map[string]any)
	require.True(t, ok, "%+v", refused.Error)
	requireBudgetRefusal(t, details)

	// Never refused, not even past the budget, nor is the call that ends one
	require.Nil(t, callWS(t, conn, 4, "subscribe", "newHeads").Error)
	require.Nil(t, callWS(t, conn, 6, "unsubscribe", subscribed.Result).Error)

	again := callWS(t, dialWS(t, url), 5, "getBlock", "1")
	require.NotNil(t, again.Error)
	assert.Equal(t, spec.ClientTimeBudgetErrorCode, again.Error.Code)
}

// graphqlMessage is one message of the graphql-transport-ws protocol
type graphqlMessage struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// dialGraphQLWS opens a GraphQL connection over WebSocket at address, in the
// graphql-transport-ws protocol, and returns once the indexer acknowledges it
func dialGraphQLWS(t *testing.T, address string) *websocket.Conn {
	t.Helper()

	conn := dialWS(t, "ws://"+address+"/graphql/query", "graphql-transport-ws")

	require.NoError(t, conn.WriteJSON(graphqlMessage{Type: "connection_init"}))

	var ack graphqlMessage

	require.NoError(t, conn.ReadJSON(&ack))
	require.Equal(t, "connection_ack", ack.Type)

	return conn
}

// subscribeGraphQL starts operation query as id over conn
func subscribeGraphQL(t *testing.T, conn *websocket.Conn, id, query string) {
	t.Helper()

	payload, err := json.Marshal(map[string]string{"query": query})
	require.NoError(t, err)

	require.NoError(t, conn.WriteJSON(graphqlMessage{ID: id, Type: "subscribe", Payload: payload}))
}

// graphqlResult is the first result of operation id over conn, read past
// every other message
func graphqlResult(t *testing.T, conn *websocket.Conn, id string) map[string]any {
	t.Helper()

	for {
		var message graphqlMessage

		require.NoError(t, conn.ReadJSON(&message))

		if message.ID != id || message.Type != "next" {
			continue
		}

		var result map[string]any

		require.NoError(t, json.Unmarshal(message.Payload, &result))

		return result
	}
}

// requireGraphQLRefusal requires result to be the time budget's refusal
func requireGraphQLRefusal(t *testing.T, result map[string]any) {
	t.Helper()

	errs, ok := result["errors"].([]any)
	require.True(t, ok, "no errors in %v", result)
	require.Len(t, errs, 1)

	refusal, ok := errs[0].(map[string]any)
	require.True(t, ok)

	details, ok := refusal["extensions"].(map[string]any)
	require.True(t, ok, "no extensions in %v", refusal)
	requireBudgetRefusal(t, details)
}

// Each query sent over GraphQL's WebSocket is charged to the client that
// opened the connection, and refused with the time budget's error once that
// client has spent it. A subscription is neither charged nor refused: it
// holds nothing while it waits for blocks. The upgrade itself is never
// refused.
func TestStartCfg_TimeBudgetChargesEachQueryOverGraphQLWebSocket(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, false, spendingBudget...)
	conn := dialGraphQLWS(t, started.apiAddress)

	query := `{ getBlocks(where: {}) { height } }`

	subscribeGraphQL(t, conn, "blocks", `subscription { getBlocks(where: {}) { height } }`)

	subscribeGraphQL(t, conn, "served", query)
	assert.NotContains(t, graphqlResult(t, conn, "served"), "errors")

	subscribeGraphQL(t, conn, "refused", query)
	requireGraphQLRefusal(t, graphqlResult(t, conn, "refused"))

	again := dialGraphQLWS(t, started.apiAddress)

	subscribeGraphQL(t, again, "again", query)
	requireGraphQLRefusal(t, graphqlResult(t, again, "again"))
}

// A GraphQL subscription holds none of its client's budget while it waits for
// blocks: with room for one request in flight, the client's queries are
// served beside it
func TestStartCfg_TimeBudgetLeavesAGraphQLSubscriptionUncharged(t *testing.T) {
	t.Parallel()

	started := startIndexer(
		t,
		false,
		"--client-time-budget", "30s",
		"--client-time-budget-refill", "30s",
		"--query-deadline", "30s",
	)
	conn := dialGraphQLWS(t, started.apiAddress)

	subscribeGraphQL(t, conn, "blocks", `subscription { getBlocks(where: {}) { height } }`)

	// The subscription starts beside the queries, so they are sent for a
	// while: a subscription charged would take the one place in flight once
	// it started
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		require.Equal(t, http.StatusOK, statuses(t, started.apiAddress, http.Header{})[0])
	}
}

// A request the time budget refuses over a WebSocket is counted as refused,
// under the API of the connection
func TestStartCfg_CountsATimeBudgetRefusalOverWebSocket(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, true, spendingBudget...)

	conn := dialWS(t, "ws://"+started.apiAddress+"/ws")

	for id := range uint(2) {
		callWS(t, conn, id+1, "getBlock", "1")
	}

	scrape := get(t, "http://"+started.metricsAddress+"/metrics")
	require.NotNil(t, scrape)

	series := refusedSeries("jsonrpc", "client_time_budget")
	assert.Equal(t, series+" 1", sample(scrape.body, series))
}
