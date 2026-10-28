package graph

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnolang/tx-indexer/events"
)

// postDocument sends query to the GraphQL endpoint of mux and answers the
// status and the body, which it never prints: an uncapped answer runs to
// hundreds of megabytes
func postDocument(t *testing.T, mux http.Handler, query string) (int, []byte) {
	t.Helper()

	body, err := json.Marshal(map[string]string{"query": query})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/graphql/query", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	answer, err := io.ReadAll(rec.Body)
	require.NoError(t, err)

	return rec.Code, answer
}

// widestConflicts is the document of at most 4000 tokens whose validation
// finds the most errors: 1,999 fields alternating latestBlockHeight and
// latestBlockHeight: __typename, each bare one in conflict with each aliased
// one, 999,000 conflicts
func widestConflicts() string {
	var b strings.Builder

	b.WriteString("{ ")

	for i := range 1999 {
		if i%2 == 0 {
			b.WriteString("latestBlockHeight ")
		} else {
			b.WriteString("latestBlockHeight: __typename ")
		}
	}

	b.WriteString("}")

	return b.String()
}

// unknownFields is a document of n fields the schema does not hold
func unknownFields(n int) string {
	var b strings.Builder

	b.WriteString("{ ")

	for i := range n {
		fmt.Fprintf(&b, "unknown%d ", i)
	}

	b.WriteString("}")

	return b.String()
}

// requireCapped requires the answer to a document that breaks one rule over
// 100 times: 422, the rule's first 100 errors, each with prefix, then one
// naming the rule and saying the rest are left out
func requireCapped(t *testing.T, mux http.Handler, query, rule, prefix string) {
	t.Helper()

	status, body := postDocument(t, mux, query)
	require.Equal(t, http.StatusUnprocessableEntity, status)

	var resp graphResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	// Not require.Len, which would print every error of an uncapped answer
	require.Equal(t, 101, len(resp.Errors))

	for _, e := range resp.Errors[:100] {
		require.True(t, strings.HasPrefix(e.Message, prefix), e.Message)
	}

	notice := resp.Errors[100]
	assert.Equal(t, "more than 100 "+rule+" errors; the rest are left out", notice.Message)
	assert.Equal(t, "GRAPHQL_VALIDATION_FAILED", notice.Extensions["code"])
}

// The widest conflicts under the default token limit, 999,000 errors of one
// rule, are answered the first 100 and one saying the rest are left out. Each
// validation counts its own: the same document sent again answers the same.
func TestValidation_CapsTheErrorsOfEachRule(t *testing.T) {
	t.Parallel()

	mux := Setup(&slowStore{}, events.NewManager(), nil, chi.NewMux(), Options{TokenLimit: 4000})

	for range 2 {
		requireCapped(t, mux, widestConflicts(), "OverlappingFieldsCanBeMerged",
			`Fields "latestBlockHeight" conflict because "latestBlockHeight" and "__typename" are different fields.`)
	}
}

// FieldsOnCorrectType, the rule gqlgen swaps for its variant with or without
// suggestions at each validation, is capped as the others are
func TestValidation_CapsTheUnknownFields(t *testing.T) {
	t.Parallel()

	mux := Setup(&slowStore{}, events.NewManager(), nil, chi.NewMux(), Options{})

	requireCapped(t, mux, unknownFields(150), "FieldsOnCorrectType", `Cannot query field "unknown`)
}

// Under the cap, an answer is the one the endpoint gives without it, byte for
// byte: a valid query's, and a document's few errors with their suggestions
func TestValidation_LeavesAnAnswerUnderTheCapAsItIs(t *testing.T) {
	t.Parallel()

	mux := Setup(&slowStore{rows: 2}, events.NewManager(), nil, chi.NewMux(), Options{TokenLimit: 4000})

	status, body := postDocument(t, mux, `{ getBlocks(where: {}) { height } }`)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, `{"data":{"getBlocks":[{"height":1},{"height":2}]}}`, string(body))

	status, body = postDocument(t, mux, `{ latestBlockHeight latestBlockHeight: __typename latestBlockHieght }`)
	assert.Equal(t, http.StatusUnprocessableEntity, status)
	assert.Equal(t,
		`{"errors":[`+
			`{"message":"Cannot query field \"latestBlockHieght\" on type \"Query\". `+
			`Did you mean \"latestBlockHeight\"?",`+
			`"locations":[{"line":1,"column":51}],"extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}},`+
			`{"message":"Fields \"latestBlockHeight\" conflict because \"latestBlockHeight\" and \"__typename\" `+
			`are different fields. Use different aliases on the fields to fetch both if this was intentional.",`+
			`"locations":[{"line":1,"column":21}],"extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}],`+
			`"data":null}`,
		string(body))
}
