package graph

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/99designs/gqlgen/client"
	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/executor"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/gqlerror"

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
	b := budget.New(budget.Config{Limit: 30 * time.Second, Refill: 30 * time.Second, Deadline: 30 * time.Second})
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

// budgetExecutor is an executor of the endpoint's schema with the budget
// extension, after exts, and a client of a budget with room for one request in
// flight
func budgetExecutor(exts ...graphql.HandlerExtension) (*executor.Executor, budget.Client) {
	b := budget.New(budget.Config{Limit: 30 * time.Second, Refill: 30 * time.Second, Deadline: 30 * time.Second})

	resolver := NewResolver(&slowStore{rows: 1}, events.NewManager(), nil)

	exec := executor.New(NewExecutableSchema(Config{Resolvers: resolver}))
	exec.Use(budgetExtension{})

	for _, ext := range exts {
		exec.Use(ext)
	}

	return exec, budget.Client{Budget: b, Key: "203.0.113.9", Address: "203.0.113.9"}
}

// admitted reports whether client is admitted a request, which it lets go of
// at once
func admitted(client budget.Client) bool {
	end, err := client.Reserve()
	if err != nil {
		return false
	}

	end()

	return true
}

// An operation keeps its place in flight until the transport asks for its
// last response, the nil one, after writing the others: a response the
// operation has made but the transport has yet to write still holds it
func TestBudgetExtension_HoldsAnOperationUntilItsLastResponse(t *testing.T) {
	t.Parallel()

	exec, charged := budgetExecutor()

	ctx := graphql.StartOperationTrace(budget.NewContext(t.Context(), charged))

	operation, errs := exec.CreateOperationContext(ctx, &graphql.RawParams{Query: `{ getBlocks(where: {}) { height } }`})
	require.Empty(t, errs)

	responses, ctx := exec.DispatchOperation(graphql.WithOperationContext(ctx, operation), operation)

	require.NotNil(t, responses(ctx))
	assert.False(t, admitted(charged), "the place was let go before the response was written")

	require.Nil(t, responses(ctx))
	assert.True(t, admitted(charged), "the place was held past the last response")
}

// panickingMutator panics in the making of every operation's context, after
// the operation's admission and before its execution
type panickingMutator struct{}

var _ graphql.OperationContextMutator = panickingMutator{}

// ExtensionName names the extension in gqlgen's stats and logs
func (panickingMutator) ExtensionName() string {
	return "Panicking"
}

// Validate accepts any schema
func (panickingMutator) Validate(graphql.ExecutableSchema) error {
	return nil
}

// MutateOperationContext panics
func (panickingMutator) MutateOperationContext(context.Context, *graphql.OperationContext) *gqlerror.Error {
	panic("the mutator failed")
}

// An operation that panics after its admission and before its execution
// never reaches the interceptors that let go of its place: it lets go of it
// once its context ends, as its connection's does, so its client is admitted
// again
func TestBudgetExtension_ReleasesAnOperationThatPanicsBeforeItsExecution(t *testing.T) {
	t.Parallel()

	exec, charged := budgetExecutor(panickingMutator{})

	connection, closeConnection := context.WithCancel(t.Context())
	ctx := graphql.StartOperationTrace(budget.NewContext(connection, charged))

	assert.PanicsWithValue(t, "the mutator failed", func() {
		_, errs := exec.CreateOperationContext(ctx, &graphql.RawParams{Query: `{ getBlocks(where: {}) { height } }`})
		assert.Empty(t, errs)
	})

	require.False(t, admitted(charged), "the operation was never admitted")

	closeConnection()

	assert.Eventually(t, func() bool {
		return admitted(charged)
	}, 5*time.Second, 10*time.Millisecond, "the client's reservation was never released")
}
