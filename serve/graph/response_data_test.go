package graph

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	abci "github.com/gnolang/gno/tm2/pkg/bft/abci/types"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnolang/tx-indexer/serve/graph/model"
)

// A transaction's response data is opaque bytes. Converted to a string as is,
// bytes that are not valid UTF-8 went straight into the JSON response, so no
// conforming client could decode any page that contained the transaction.
func TestGetTransactions_InvalidUTF8ResponseDataIsValidJSON(t *testing.T) {
	t.Parallel()

	r := testResolver(t)

	wb := r.store.WriteBatch()
	require.NoError(t, wb.SetTx(&types.TxResult{
		Height: 1,
		Tx:     types.Tx{0x01},
		Response: abci.ResponseDeliverTx{
			ResponseBase: abci.ResponseBase{
				Data: []byte("ok \xff\xfe end"),
			},
		},
	}))
	require.NoError(t, wb.Commit())

	srv := handler.New(NewExecutableSchema(Config{
		Resolvers: r,
		Directives: DirectiveRoot{
			Filterable: func(
				ctx context.Context,
				_ interface{},
				next graphql.Resolver,
				_ []model.FilterableExtra,
			) (interface{}, error) {
				return next(ctx)
			},
		},
	}))
	srv.AddTransport(transport.POST{})

	body, err := json.Marshal(map[string]string{
		"query": `{ getTransactions(where: {}) { response { data } } }`,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	raw := rec.Body.Bytes()
	require.True(t, utf8.Valid(raw), "response is not valid UTF-8: %q", raw)
	require.True(t, json.Valid(raw), "response is not valid JSON: %q", raw)

	var resp struct {
		Data struct {
			GetTransactions []struct {
				Response struct {
					Data string `json:"data"`
				} `json:"response"`
			} `json:"getTransactions"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &resp))
	require.Len(t, resp.Data.GetTransactions, 1)

	// The valid parts of the data survive; the invalid run becomes U+FFFD.
	assert.Equal(t, "ok � end", resp.Data.GetTransactions[0].Response.Data)
}
