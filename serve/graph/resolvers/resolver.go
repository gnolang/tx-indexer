//go:generate go run github.com/99designs/gqlgen generate

package resolvers

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

type Resolver struct {
	store   storage.Storage
	manager *events.Manager
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

func NewResolver(s storage.Storage, m *events.Manager) *Resolver {
	return &Resolver{store: s, manager: m}
}
