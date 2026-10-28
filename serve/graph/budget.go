package graph

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/gnolang/tx-indexer/metrics"
	"github.com/gnolang/tx-indexer/serve/budget"
	"github.com/gnolang/tx-indexer/serve/observe"
)

// budgetExtension charges each operation carried over a WebSocket connection
// to the client that opened it, when a time budget admitted the connection,
// and refuses it with the budget's error once that client has spent it. The
// operation is admitted, and holds its place in flight, from before its
// document is parsed: parsing and validating it are charged, and a client over
// its budget has nothing parsed. A subscription lets go of its place once its
// document is validated, and its stream is never charged: it lives as long as
// its connection. An operation over HTTP carries no client: its request was
// admitted whole before it reached the endpoint.
type budgetExtension struct {
	// observer records each refusal; nil records none
	observer *observe.Observer
}

var (
	_ graphql.HandlerExtension          = budgetExtension{}
	_ graphql.OperationParameterMutator = budgetExtension{}
	_ graphql.OperationContextMutator   = budgetExtension{}
	_ graphql.OperationInterceptor      = budgetExtension{}
	_ graphql.ResponseInterceptor       = budgetExtension{}
)

// reservationKey names an operation's reservation among its stats'
// extensions
const reservationKey = "ClientTimeBudget"

// reservation is what an operation over a WebSocket holds of its client's
// budget, from before its document is parsed
type reservation struct {
	// end lets go of the place in flight and charges the time since the
	// operation's admission
	end func()

	// stop unregisters the release at the end of the context the operation
	// was admitted under, once the reservation is released; nil until the
	// release is registered
	stop atomic.Pointer[func() bool]

	// ended makes end run once, whichever way the operation ends first
	ended sync.Once

	// dispatched reports whether the operation reached its execution, which
	// lets go of the reservation itself
	dispatched atomic.Bool
}

// release ends the reservation, once, and unregisters its release at the end
// of the context the operation was admitted under
func (r *reservation) release() {
	r.ended.Do(func() {
		if stop := r.stop.Load(); stop != nil {
			(*stop)()
		}

		r.end()
	})
}

// reservationOf is the reservation the operation of ctx holds, nil for none
func reservationOf(ctx context.Context) *reservation {
	if !graphql.HasOperationContext(ctx) {
		return nil
	}

	held, _ := graphql.GetOperationContext(ctx).Stats.GetExtension(reservationKey).(*reservation)

	return held
}

// ExtensionName names the extension in gqlgen's stats and logs
func (budgetExtension) ExtensionName() string {
	return reservationKey
}

// Validate accepts any schema: the budget does not depend on it
func (budgetExtension) Validate(graphql.ExecutableSchema) error {
	return nil
}

// MutateOperationParameters admits an operation of the connection's client
// before its document is parsed, and keeps the reservation in the operation's
// stats; or refuses it, with the budget's error, which the transport answers
// once. An operation that fails between its admission and its execution, as
// one that panics there, reaches none of the interceptors that release it: it
// is released once the context it was admitted under ends, with its
// connection's at the latest.
func (e budgetExtension) MutateOperationParameters(ctx context.Context, _ *graphql.RawParams) *gqlerror.Error {
	client, ok := budget.FromContext(ctx)
	if !ok {
		return nil
	}

	end, err := client.Reserve()
	if err != nil {
		var exhausted *budget.ExhaustedError
		if errors.As(err, &exhausted) {
			e.observer.Refused(client.Address, metrics.APIGraphQL, metrics.RefusalClientTimeBudget, exhausted.RetryAfter)
		}

		return refusalError(err)
	}

	held := &reservation{end: end}

	stop := context.AfterFunc(ctx, held.release)
	held.stop.Store(&stop)

	graphql.GetOperationContext(ctx).Stats.SetExtension(reservationKey, held)

	return nil
}

// MutateOperationContext lets a subscription go of its place in flight once
// its document is validated, charging the parsing and the validating
func (budgetExtension) MutateOperationContext(ctx context.Context, opCtx *graphql.OperationContext) *gqlerror.Error {
	if opCtx.Operation == nil || opCtx.Operation.Operation != ast.Subscription {
		return nil
	}

	if held := reservationOf(ctx); held != nil {
		held.release()
	}

	return nil
}

// InterceptOperation keeps an operation's reservation until its last response
// has been sent: the transport asks for responses until one is nil. An
// operation that never gets there, as one that panics outside its resolvers,
// lets go of it once its context ends, when the transport is done with it or
// at the latest with its connection.
func (budgetExtension) InterceptOperation(
	ctx context.Context,
	next graphql.OperationHandler,
) graphql.ResponseHandler {
	held := reservationOf(ctx)
	if held == nil {
		return next(ctx)
	}

	held.dispatched.Store(true)

	stop := context.AfterFunc(ctx, held.release)

	responses := next(ctx)

	return func(ctx context.Context) *graphql.Response {
		response := responses(ctx)
		if response == nil {
			stop()
			held.release()
		}

		return response
	}
}

// InterceptResponse lets go of the reservation of an operation that failed
// before its execution, its document invalid among such failures: the
// transport answers the failure through the response interceptors, and the
// operation never reaches its own
func (budgetExtension) InterceptResponse(ctx context.Context, next graphql.ResponseHandler) *graphql.Response {
	response := next(ctx)

	if held := reservationOf(ctx); held != nil && !held.dispatched.Load() {
		held.release()
	}

	return response
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
