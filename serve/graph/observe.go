package graph

import (
	"context"

	"github.com/99designs/gqlgen/graphql"

	"github.com/gnolang/tx-indexer/internal/tally"
	"github.com/gnolang/tx-indexer/metrics"
	"github.com/gnolang/tx-indexer/serve/graph/model"
	"github.com/gnolang/tx-indexer/serve/observe"
)

// observeExtension records each root field of a query as one request of the
// GraphQL API, named after the field: its count, duration, status and rows.
// Subscriptions are left out: they last as long as their connection, and
// gqlgen runs no root field interceptor for them.
type observeExtension struct {
	observer *observe.Observer
}

var (
	_ graphql.HandlerExtension     = observeExtension{}
	_ graphql.RootFieldInterceptor = observeExtension{}
	_ graphql.FieldInterceptor     = observeExtension{}
)

// rootAnswer is what the resolver of a root field answered, noted by
// InterceptField, which sees it, for InterceptRootField, which records it
type rootAnswer struct {
	// err is the error the resolver returned, nil for none
	err error
}

// rootAnswerKey is the context key a rootAnswer is carried under
type rootAnswerKey struct{}

// ExtensionName names the extension in gqlgen's stats and logs
func (observeExtension) ExtensionName() string {
	return "Observe"
}

// Validate accepts any schema: observing does not depend on it
func (observeExtension) Validate(graphql.ExecutableSchema) error {
	return nil
}

// InterceptRootField observes a root field over its whole run: its resolver,
// and the fields of the rows it answers
func (e observeExtension) InterceptRootField(
	ctx context.Context,
	next graphql.RootResolver,
) graphql.Marshaler {
	ctx, call := e.observer.Start(ctx, metrics.APIGraphQL, graphql.GetRootFieldContext(ctx).Field.Name)

	// The field stays unanswered until its resolver returns: a field whose
	// arguments do not coerce never reaches its resolver, and a resolver that
	// panics never returns, gqlgen recovering it below
	answer := &rootAnswer{err: observe.ErrUnanswered}
	marshaler := next(context.WithValue(ctx, rootAnswerKey{}, answer))

	call.End(ctx, answer.err)

	return marshaler
}

// InterceptField notes what the resolver of a root field answered: its error,
// and the rows of its list
func (observeExtension) InterceptField(ctx context.Context, next graphql.Resolver) (any, error) {
	answer := rootAnswerOf(ctx)
	if answer == nil {
		return next(ctx)
	}

	res, err := next(ctx)

	answer.err = err

	tally.FromContext(ctx).AddReturned(answeredRows(res))

	return res, err
}

// rootAnswerOf returns the answer of the root field ctx resolves, or nil when
// ctx resolves a field below one, such as a transaction's hash, whose context
// carries the answer of its root field all the same
func rootAnswerOf(ctx context.Context) *rootAnswer {
	if graphql.GetFieldContext(ctx).Object != "Query" {
		return nil
	}

	answer, _ := ctx.Value(rootAnswerKey{}).(*rootAnswer)

	return answer
}

// answeredRows counts the rows a root field's resolver answered: the blocks or
// transactions of its list. Any other answer, such as a height, is no row.
func answeredRows(res any) int64 {
	switch rows := res.(type) {
	case []*model.Transaction:
		return int64(len(rows))
	case []*model.Block:
		return int64(len(rows))
	default:
		return 0
	}
}
