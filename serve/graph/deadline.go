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

// scanError is the error a list resolver answers when err cuts its scan short,
// whether the resolver saw its context end between two rows or the storage
// stopped on it in a walk past rows outside the index window. The query
// deadline names itself in the error's extensions; any other error, such as
// the client leaving, is answered as it is.
//
// A transaction list answers it with no rows: the rows read so far can end
// inside a block and say nothing of how far the scan got, so the client could
// neither resume from them nor tell them from a whole page. A block list
// answers it beside the blocks read so far: a block row is a whole block, so
// the client resumes from the last height.
func scanError(err error) error {
	var exceeded *deadline.ExceededError
	if !errors.As(err, &exceeded) {
		return err
	}

	return &gqlerror.Error{
		Err:        err,
		Message:    err.Error(),
		Extensions: exceeded.Details(),
	}
}
