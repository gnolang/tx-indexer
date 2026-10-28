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

// dialWS opens a WebSocket connection to url, its upgrade request carrying
// header, in subprotocol, if any, and closes it when the test ends
func dialWS(t *testing.T, url string, header http.Header, subprotocols ...string) *websocket.Conn {
	t.Helper()

	dialer := websocket.Dialer{Subprotocols: subprotocols, HandshakeTimeout: 5 * time.Second}

	conn, resp, err := dialer.DialContext(t.Context(), url, header)
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

	conn := dialWS(t, url, nil)

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

	again := callWS(t, dialWS(t, url, nil), 5, "getBlock", "1")
	require.NotNil(t, again.Error)
	assert.Equal(t, spec.ClientTimeBudgetErrorCode, again.Error.Code)
}

// blocksQuery is a GraphQL query of every block's height
const blocksQuery = `{ getBlocks(where: {}) { height } }`

// graphqlMessage is one message of the graphql-transport-ws protocol, or of
// the legacy graphql-ws, whose messages share its shape
type graphqlMessage struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// dialGraphQLWS opens a GraphQL connection over WebSocket at address, its
// upgrade request carrying header, in the graphql-transport-ws protocol, and
// returns once the indexer acknowledges it
func dialGraphQLWS(t *testing.T, address string, header http.Header) *websocket.Conn {
	t.Helper()

	return dialGraphQLWSIn(t, address, header, "graphql-transport-ws")
}

// dialGraphQLWSIn opens a GraphQL connection over WebSocket at address, its
// upgrade request carrying header, in subprotocol, and returns once the
// indexer acknowledges it
func dialGraphQLWSIn(t *testing.T, address string, header http.Header, subprotocol string) *websocket.Conn {
	t.Helper()

	conn := dialWS(t, "ws://"+address+"/graphql/query", header, subprotocol)

	require.NoError(t, conn.WriteJSON(graphqlMessage{Type: "connection_init"}))

	var ack graphqlMessage

	require.NoError(t, conn.ReadJSON(&ack))
	require.Equal(t, "connection_ack", ack.Type)

	return conn
}

// subscribeGraphQL starts operation query as id over conn
func subscribeGraphQL(t *testing.T, conn *websocket.Conn, id, query string) {
	t.Helper()

	startGraphQL(t, conn, "subscribe", id, query)
}

// startGraphQL starts operation query as id over conn, in a message of
// messageType, the one its protocol starts an operation with
func startGraphQL(t *testing.T, conn *websocket.Conn, messageType, id, query string) {
	t.Helper()

	payload, err := json.Marshal(map[string]string{"query": query})
	require.NoError(t, err)

	require.NoError(t, conn.WriteJSON(graphqlMessage{ID: id, Type: messageType, Payload: payload}))
}

// graphqlResult is the one result of operation id over conn, which must
// complete after it, read past every other operation's messages
func graphqlResult(t *testing.T, conn *websocket.Conn, id string) map[string]any {
	t.Helper()

	var results []map[string]any

	for {
		var message graphqlMessage

		require.NoError(t, conn.ReadJSON(&message))

		if message.ID != id {
			continue
		}

		if message.Type == "complete" {
			require.Len(t, results, 1, "operation %s answered %d results", id, len(results))

			return results[0]
		}

		require.Equal(t, "next", message.Type, string(message.Payload))
		require.Empty(t, results, "operation %s answered a second result", id)

		var result map[string]any

		require.NoError(t, json.Unmarshal(message.Payload, &result))

		results = append(results, result)
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
// client has spent it. The upgrade itself is never refused.
func TestStartCfg_TimeBudgetChargesEachQueryOverGraphQLWebSocket(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, false, spendingBudget...)
	conn := dialGraphQLWS(t, started.apiAddress, nil)

	query := blocksQuery

	subscribeGraphQL(t, conn, "served", query)
	assert.NotContains(t, graphqlResult(t, conn, "served"), "errors")

	subscribeGraphQL(t, conn, "refused", query)
	requireGraphQLRefusal(t, graphqlResult(t, conn, "refused"))

	again := dialGraphQLWS(t, started.apiAddress, nil)

	subscribeGraphQL(t, again, "again", query)
	requireGraphQLRefusal(t, graphqlResult(t, again, "again"))
}

// A GraphQL subscription holds none of its client's budget while it waits for
// blocks, once started: with room for one request in flight, the client's
// queries are served beside it
func TestStartCfg_TimeBudgetLeavesAGraphQLSubscriptionUncharged(t *testing.T) {
	t.Parallel()

	started := startIndexer(
		t,
		false,
		"--client-time-budget", "30s",
		"--client-time-budget-refill", "30s",
		"--query-deadline", "30s",
	)
	conn := dialGraphQLWS(t, started.apiAddress, nil)

	subscribeGraphQL(t, conn, "blocks", `subscription { getBlocks(where: {}) { height } }`)

	// The connection reads its messages in turn: a query after the
	// subscription is read once the subscription has started, and served
	// only if the subscription's start let go of the one place in flight
	subscribeGraphQL(t, conn, "query", blocksQuery)
	assert.NotContains(t, graphqlResult(t, conn, "query"), "errors")

	// A subscription that took the place once started would refuse every
	// query sent beside it
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		require.Equal(t, http.StatusOK, statuses(t, started.apiAddress, http.Header{})[0])
	}
}

// A query or call the time budget refuses over a WebSocket is counted as
// refused, under the API of the connection
func TestStartCfg_CountsATimeBudgetRefusalOverWebSocket(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, true, spendingBudget...)

	conn := dialWS(t, "ws://"+started.apiAddress+"/ws", nil)

	for id := range uint(2) {
		callWS(t, conn, id+1, "getBlock", "1")
	}

	graphql := dialGraphQLWS(t, started.apiAddress, nil)

	subscribeGraphQL(t, graphql, "refused", blocksQuery)
	requireGraphQLRefusal(t, graphqlResult(t, graphql, "refused"))

	scrape := get(t, "http://"+started.metricsAddress+"/metrics")
	require.NotNil(t, scrape)

	for _, api := range []string{"jsonrpc", "graphql"} {
		series := refusedSeries(api, "client_time_budget")
		assert.Equal(t, series+" 1", sample(scrape.body, series))
	}
}

// Two clients behind the trusted proxy are charged apart over a WebSocket too,
// each under the address the proxy names in its connection's upgrade
func TestStartCfg_TimeBudgetChargesWebSocketClientsApart(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, false, append(spendingBudget, "--client-ip-header", "CF-Connecting-IP")...)
	client := func(address string) http.Header {
		return http.Header{"Cf-Connecting-Ip": {address}}
	}

	url := "ws://" + started.apiAddress + "/ws"

	spent := dialWS(t, url, client("203.0.113.9"))
	callWS(t, spent, 1, "getBlock", "1")

	refused := callWS(t, spent, 2, "getBlock", "1")
	require.NotNil(t, refused.Error)
	assert.Equal(t, spec.ClientTimeBudgetErrorCode, refused.Error.Code)

	served := callWS(t, dialWS(t, url, client("203.0.113.10")), 3, "getBlock", "1")
	if served.Error != nil {
		assert.NotEqual(t, spec.ClientTimeBudgetErrorCode, served.Error.Code)
	}

	query := blocksQuery

	spentGraphQL := dialGraphQLWS(t, started.apiAddress, client("203.0.113.11"))

	subscribeGraphQL(t, spentGraphQL, "served", query)
	assert.NotContains(t, graphqlResult(t, spentGraphQL, "served"), "errors")

	subscribeGraphQL(t, spentGraphQL, "refused", query)
	requireGraphQLRefusal(t, graphqlResult(t, spentGraphQL, "refused"))

	other := dialGraphQLWS(t, started.apiAddress, client("203.0.113.12"))

	subscribeGraphQL(t, other, "other", query)
	assert.NotContains(t, graphqlResult(t, other, "other"), "errors")
}

// graphqlMessages are the messages of operation id over conn up to its
// completion, read past every other operation's messages
func graphqlMessages(t *testing.T, conn *websocket.Conn, id string) []graphqlMessage {
	t.Helper()

	var messages []graphqlMessage

	for {
		var message graphqlMessage

		require.NoError(t, conn.ReadJSON(&message))

		if message.ID != id {
			continue
		}

		if message.Type == "complete" {
			return messages
		}

		messages = append(messages, message)
	}
}

// A client over its budget is refused before its document is parsed and
// validated, over a WebSocket: an invalid document answers the budget's
// error, not a validation error
func TestStartCfg_TimeBudgetRefusesBeforeValidating(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, false, spendingBudget...)
	conn := dialGraphQLWS(t, started.apiAddress, nil)

	subscribeGraphQL(t, conn, "served", blocksQuery)
	assert.NotContains(t, graphqlResult(t, conn, "served"), "errors")

	subscribeGraphQL(t, conn, "invalid", `{ notAField }`)
	requireGraphQLRefusal(t, graphqlResult(t, conn, "invalid"))
}

// Parsing and validating a document is charged to its client over a
// WebSocket, a document that fails validation included, and the operation's
// reservation is let go once it fails
func TestStartCfg_TimeBudgetChargesAnInvalidDocument(t *testing.T) {
	t.Parallel()

	t.Run("its validation is charged", func(t *testing.T) {
		t.Parallel()

		started := startIndexer(t, false, spendingBudget...)
		conn := dialGraphQLWS(t, started.apiAddress, nil)

		subscribeGraphQL(t, conn, "invalid", `{ notAField }`)

		invalid := graphqlMessages(t, conn, "invalid")
		require.Len(t, invalid, 1)
		assert.Equal(t, "error", invalid[0].Type, string(invalid[0].Payload))

		subscribeGraphQL(t, conn, "refused", blocksQuery)
		requireGraphQLRefusal(t, graphqlResult(t, conn, "refused"))
	})

	t.Run("its reservation is let go", func(t *testing.T) {
		t.Parallel()

		// Room for one request in flight
		started := startIndexer(
			t,
			false,
			"--client-time-budget", "30s",
			"--client-time-budget-refill", "30s",
			"--query-deadline", "30s",
		)
		conn := dialGraphQLWS(t, started.apiAddress, nil)

		subscribeGraphQL(t, conn, "invalid", `{ notAField }`)
		require.Len(t, graphqlMessages(t, conn, "invalid"), 1)

		subscribeGraphQL(t, conn, "served", blocksQuery)
		assert.NotContains(t, graphqlResult(t, conn, "served"), "errors")
	})
}

// A subscription's start parses and validates its document, so a client over
// its budget is refused one, as any other operation
func TestStartCfg_TimeBudgetRefusesASubscriptionsStart(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, false, spendingBudget...)
	conn := dialGraphQLWS(t, started.apiAddress, nil)

	subscribeGraphQL(t, conn, "served", blocksQuery)
	assert.NotContains(t, graphqlResult(t, conn, "served"), "errors")

	subscribeGraphQL(t, conn, "blocks", `subscription { getBlocks(where: {}) { height } }`)
	requireGraphQLRefusal(t, graphqlResult(t, conn, "blocks"))
}

// A GraphQL document over the token limit is refused before it is validated,
// over a WebSocket too
func TestStartCfg_RefusesAGraphQLDocumentOverTheTokenLimitOverWebSocket(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, false, "--graphql-token-limit", "10")
	conn := dialGraphQLWS(t, started.apiAddress, nil)

	subscribeGraphQL(t, conn, "served", `{ __typename }`)
	assert.NotContains(t, graphqlResult(t, conn, "served"), "errors")

	subscribeGraphQL(t, conn, "refused", blocksQuery)

	refused := graphqlMessages(t, conn, "refused")
	require.Len(t, refused, 1)
	assert.Equal(t, "error", refused[0].Type)
	assert.Contains(t, string(refused[0].Payload), "exceeded token limit of 10")
}

// A call over JSON-RPC's WebSocket lets go of its place in flight once
// answered: with room for one request in flight, the client's calls are
// served one after the other
func TestStartCfg_TimeBudgetReleasesEachCallOverJSONRPCWebSocket(t *testing.T) {
	t.Parallel()

	started := startIndexer(
		t,
		false,
		"--client-time-budget", "30s",
		"--client-time-budget-refill", "30s",
		"--query-deadline", "30s",
	)
	conn := dialWS(t, "ws://"+started.apiAddress+"/ws", nil)

	for id := range uint(3) {
		answer := callWS(t, conn, id+1, "getBlock", "1")
		if answer.Error != nil {
			assert.NotEqual(t, spec.ClientTimeBudgetErrorCode, answer.Error.Code, "call %d", id+1)
		}
	}
}

// The legacy graphql-ws protocol, which gqlgen still speaks, answers an
// operation the time budget refuses as graphql-transport-ws does: one data
// message holding the budget's error, then the operation's completion
func TestStartCfg_TimeBudgetRefusesOverTheLegacyGraphQLWSProtocol(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, false, spendingBudget...)
	conn := dialGraphQLWSIn(t, started.apiAddress, nil, "graphql-ws")

	query := blocksQuery

	startGraphQL(t, conn, "start", "served", query)

	served := graphqlMessages(t, conn, "served")
	require.Len(t, served, 1)
	assert.Equal(t, "data", served[0].Type)
	assert.NotContains(t, string(served[0].Payload), "errors")

	startGraphQL(t, conn, "start", "refused", query)

	refused := graphqlMessages(t, conn, "refused")
	require.Len(t, refused, 1)
	require.Equal(t, "data", refused[0].Type)

	var result map[string]any

	require.NoError(t, json.Unmarshal(refused[0].Payload, &result))
	requireGraphQLRefusal(t, result)
}

// A persisted query over a WebSocket is admitted before gqlgen's APQ reads
// it: a client over its budget is refused with the budget's error, counted as
// refused, whatever its persisted query holds, a hash that does not match
// included
func TestStartCfg_TimeBudgetRefusesAPersistedQueryOverWebSocket(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, true, spendingBudget...)
	conn := dialGraphQLWS(t, started.apiAddress, nil)

	subscribeGraphQL(t, conn, "served", blocksQuery)
	assert.NotContains(t, graphqlResult(t, conn, "served"), "errors")

	payload, err := json.Marshal(map[string]any{
		"query":      blocksQuery,
		"extensions": map[string]any{"persistedQuery": map[string]any{"version": 1, "sha256Hash": "00"}},
	})
	require.NoError(t, err)

	require.NoError(t, conn.WriteJSON(graphqlMessage{ID: "persisted", Type: "subscribe", Payload: payload}))
	requireGraphQLRefusal(t, graphqlResult(t, conn, "persisted"))

	scrape := get(t, "http://"+started.metricsAddress+"/metrics")
	require.NotNil(t, scrape)

	series := refusedSeries("graphql", "client_time_budget")
	assert.Equal(t, series+" 1", sample(scrape.body, series))
}
