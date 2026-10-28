package graph

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/gnolang/tx-indexer/metrics"
	"github.com/gnolang/tx-indexer/serve/budget"
	"github.com/gnolang/tx-indexer/serve/observe"
)

// budgetExtension charges each query carried over a WebSocket connection to
// the client that opened it, when a time budget admitted the connection, and
// refuses it with the budget's error once that client has spent it. A
// subscription is neither charged nor refused: it lives as long as its
// connection. A query over HTTP carries no client: its request was charged
// whole before it reached the endpoint.
type budgetExtension struct {
	// observer records each refusal; nil records none
	observer *observe.Observer
}

var (
	_ graphql.HandlerExtension     = budgetExtension{}
	_ graphql.OperationInterceptor = budgetExtension{}
)

// ExtensionName names the extension in gqlgen's stats and logs
func (budgetExtension) ExtensionName() string {
	return "ClientTimeBudget"
}

// Validate accepts any schema: the budget does not depend on it
func (budgetExtension) Validate(graphql.ExecutableSchema) error {
	return nil
}

// InterceptOperation charges an operation to the client of its connection,
// from its admission until its last response has been sent: the transport
// asks for responses until one is nil. A refused operation answers the
// budget's error once.
func (e budgetExtension) InterceptOperation(
	ctx context.Context,
	next graphql.OperationHandler,
) graphql.ResponseHandler {
	if isSubscription(ctx) {
		return next(ctx)
	}

	client, ok := budget.FromContext(ctx)
	if !ok {
		return next(ctx)
	}

	end, err := client.Reserve()
	if err != nil {
		var exhausted *budget.ExhaustedError
		if errors.As(err, &exhausted) {
			e.observer.Refused(client.Address, metrics.APIGraphQL, metrics.RefusalClientTimeBudget, exhausted.RetryAfter)
		}

		return graphql.OneShot(&graphql.Response{Errors: gqlerror.List{refusalError(err)}})
	}

	responses := next(ctx)

	var ended sync.Once

	return func(ctx context.Context) *graphql.Response {
		response := responses(ctx)
		if response == nil {
			ended.Do(end)
		}

		return response
	}
}

// WriteRefusal answers a GraphQL request refused before it was served with a
// 429 and err as its one error, in the shape the query deadline's takes: its
// message, and its details under extensions, with no data
func WriteRefusal(w http.ResponseWriter, err error) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)

	return json.NewEncoder(w).Encode(&graphql.Response{Errors: gqlerror.List{refusalError(err)}})
}

// refusalError is err as a GraphQL error, with the details of a time budget
// refusal under extensions
func refusalError(err error) *gqlerror.Error {
	refusal := &gqlerror.Error{Err: err, Message: err.Error()}

	var exhausted *budget.ExhaustedError
	if errors.As(err, &exhausted) {
		refusal.Extensions = exhausted.Details()
	}

	return refusal
}
