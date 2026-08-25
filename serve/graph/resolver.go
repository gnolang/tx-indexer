//go:generate go run gen/generate.go

package graph

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"

	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/storage"
	"github.com/gnolang/tx-indexer/types"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

const maxElementsPerQuery = 10000

// effectiveLimit resolves the caller's `limit` against the hard cap.
//
// Without one, a query that matches many rows returns up to maxElementsPerQuery
// of them — 10,000 transactions a caller usually did not want and has to
// transfer anyway. A page of twenty is the common case, and asking for twenty
// should cost twenty.
//
// The cap stays the ceiling: limit only ever lowers it. A non-positive limit is
// treated as absent rather than rejected, so `limit: 0` behaves as it does today
// instead of silently returning nothing.
func effectiveLimit(limit *int) int {
	if limit == nil || *limit <= 0 || *limit > maxElementsPerQuery {
		return maxElementsPerQuery
	}
	return *limit
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
}

func NewResolver(s storage.Storage, m *events.Manager) *Resolver {
	return &Resolver{store: s, manager: m}
}
