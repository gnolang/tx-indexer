package graph

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/99designs/gqlgen/client"
	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/serve/budget"
)

// failingExtension panics in the making of every response, outside any
// resolver, as an extension that fails would
type failingExtension struct{}

var _ graphql.ResponseInterceptor = failingExtension{}

// ExtensionName names the extension in gqlgen's stats and logs
func (failingExtension) ExtensionName() string {
	return "Failing"
}

// Validate accepts any schema
func (failingExtension) Validate(graphql.ExecutableSchema) error {
	return nil
}

// InterceptResponse panics
func (failingExtension) InterceptResponse(context.Context, graphql.ResponseHandler) *graphql.Response {
	panic("the extension failed")
}

// An operation over a WebSocket that panics outside its resolvers never
// returns its last response: its reservation is still released once the
// operation ends, at the latest with its connection, so its client is admitted
// again
func TestBudgetExtension_ReleasesAPanickingOperation(t *testing.T) {
	t.Parallel()

	// Room for one request in flight
	b := budget.New(30*time.Second, 30*time.Second, 30*time.Second)
	charged := budget.Client{Budget: b, Key: "203.0.113.9", Address: "203.0.113.9"}

	srv := handler.New(NewExecutableSchema(Config{Resolvers: NewResolver(&slowStore{}, events.NewManager(), nil)}))
	srv.AddTransport(transport.Websocket{})

	// The panic is answered as an error, without the stack gqlgen prints by
	// default
	srv.SetRecoverFunc(func(_ context.Context, recovered any) error {
		return fmt.Errorf("recovered: %v", recovered)
	})

	srv.Use(budgetExtension{})
	srv.Use(failingExtension{})

	opened := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.ServeHTTP(w, r.WithContext(budget.NewContext(r.Context(), charged)))
	})

	operation := client.New(opened).Websocket(`{ getBlocks(where: {}) { height } }`)

	var response map[string]any

	require.Error(t, operation.Next(&response), "the panic was not answered as an error")
	require.NoError(t, operation.Close())

	assert.Eventually(t, func() bool {
		end, err := b.Reserve(charged.Key)
		if err != nil {
			return false
		}

		end()

		return true
	}, 5*time.Second, 10*time.Millisecond, "the client's reservation was never released")
}
