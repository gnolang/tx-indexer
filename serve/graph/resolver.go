//go:generate go run gen/generate.go

package graph

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/serve/handlers/supply"
	"github.com/gnolang/tx-indexer/storage"
	"github.com/gnolang/tx-indexer/types"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

const maxElementsPerQuery = 10000

// effectiveLimit resolves the caller's `limit` against the hard cap. It
// returns how many rows to read and whether that bound is the cap, the only
// bound whose reach is reported as a truncation.
//
// Without one, a query that matches many rows returns up to maxElementsPerQuery
// of them, rows a caller usually did not want and has to transfer anyway. A
// page of twenty is the common case, and asking for twenty should cost twenty.
//
// The cap stays the ceiling: limit only ever lowers it, and a limit above it
// is capped as with no limit. A limit below 1 is rejected: it can only be a
// caller's mistake, and treating it as absent would answer it with the cap.
func effectiveLimit(limit *int) (int, bool, error) {
	switch {
	case limit == nil, *limit > maxElementsPerQuery:
		return maxElementsPerQuery, true, nil
	case *limit < 1:
		return 0, false, gqlerror.Errorf("limit must be at least 1, got %d", *limit)
	default:
		return *limit, false, nil
	}
}

func deref[T any](v *T) T {
	if v == nil {
		var zero T

		return zero
	}

	return *v
}

func handleChannel[T any](
	ctx context.Context,
	m *events.Manager,
	writeToChannel func(*types.NewBlock, chan<- T),
) <-chan T {
	ch := make(chan T)

	go func() {
		defer close(ch)

		sub := m.Subscribe([]events.Type{types.NewBlockEvent})
		defer m.CancelSubscription(sub.ID)

		// gqlgen's resolver recovery does not cover this goroutine, so an unrecovered
		// panic here would exit the process instead of ending this subscription.
		// Recover is called for its logging: errors added here never reach the client.
		defer func() {
			if r := recover(); r != nil {
				graphql.AddError(ctx, graphql.GetOperationContext(ctx).Recover(ctx, r))
			}
		}()

		for {
			select {
			case <-ctx.Done():
				graphql.AddError(ctx, ctx.Err())

				return
			case rawE, ok := <-sub.SubCh:
				if !ok {
					return
				}

				e, ok := rawE.GetData().(*types.NewBlock)
				if !ok {
					graphql.AddError(ctx, fmt.Errorf("error casting event data. Obtained event ID: %q", rawE.GetType()))

					return
				}

				writeToChannel(e, ch)
			}
		}
	}()

	return ch
}

type Resolver struct {
	store   storage.Storage
	manager *events.Manager
	supply  *supply.Handler
}

func NewResolver(s storage.Storage, m *events.Manager, sup *supply.Handler) *Resolver {
	return &Resolver{store: s, manager: m, supply: sup}
}
