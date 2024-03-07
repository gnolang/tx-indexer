//go:generate go run github.com/99designs/gqlgen generate

package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/99designs/gqlgen/graphql"

	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/storage"
	"github.com/gnolang/tx-indexer/types"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

const maxElementsPerQuery = 10000

func dereferenceInt(i *int) int {
	if i == nil {
		return 0
	}

	return *i
}

func dereferenceTime(i *time.Time) time.Time {
	if i == nil {
		var t time.Time

		return t
	}

	return *i
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
