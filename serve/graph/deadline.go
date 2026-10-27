package graph

import (
	"context"
	"errors"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/gnolang/tx-indexer/serve/deadline"
)

// deadlineExtension bounds every query by the query deadline, whichever
// transport carries it. Subscriptions are left unbounded: they live as long as
// their connection, and between two events gqlgen waits on the very context
// the deadline would end.
type deadlineExtension struct {
	deadline time.Duration
}

var (
	_ graphql.HandlerExtension    = deadlineExtension{}
	_ graphql.ResponseInterceptor = deadlineExtension{}
)

// ExtensionName names the extension in gqlgen's stats and logs
func (deadlineExtension) ExtensionName() string {
	return "QueryDeadline"
}

// Validate accepts any schema: the deadline does not depend on it
func (deadlineExtension) Validate(graphql.ExecutableSchema) error {
	return nil
}

// InterceptResponse bounds the making of a query's response, the step in
// which its resolvers run, by the query deadline
func (e deadlineExtension) InterceptResponse(
	ctx context.Context,
	next graphql.ResponseHandler,
) *graphql.Response {
	if isSubscription(ctx) {
		return next(ctx)
	}

	ctx, cancel := deadline.WithTimeout(ctx, e.deadline)
	defer cancel()

	return next(ctx)
}

// isSubscription reports whether ctx carries a subscription operation. A
// request that failed before naming its operation carries none.
func isSubscription(ctx context.Context) bool {
	if !graphql.HasOperationContext(ctx) {
		return false
	}

	operation := graphql.GetOperationContext(ctx).Operation

	return operation != nil && operation.Operation == ast.Subscription
}

// scanStopped is what a list resolver answers when its context ends before its
// scan does. It answers no rows: the rows read so far can end inside a block
// and say nothing of how far the scan got, so the client could neither resume
// from them nor tell them from a whole page. The query deadline names itself
// in the error's extensions; any other end, such as the client leaving,
// answers the context's cause as it is.
func scanStopped(ctx context.Context) error {
	cause := context.Cause(ctx)

	var exceeded *deadline.ExceededError
	if !errors.As(cause, &exceeded) {
		return cause
	}

	return &gqlerror.Error{
		Err:        cause,
		Message:    cause.Error(),
		Extensions: exceeded.Details(),
	}
}
