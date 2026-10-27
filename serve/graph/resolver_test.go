package graph

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/99designs/gqlgen/graphql"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/stretchr/testify/require"

	"github.com/gnolang/tx-indexer/events"
	indexerTypes "github.com/gnolang/tx-indexer/types"
)

func TestHandleChannel_PanicEndsOnlyItsSubscription(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var recovered any

		capture := func(_ context.Context, r any) error {
			recovered = r

			return errors.New("recovered")
		}

		ctx := graphql.WithOperationContext(t.Context(), &graphql.OperationContext{RecoverFunc: capture})
		ctx = graphql.WithResponseContext(ctx, graphql.DefaultErrorPresenter, capture)

		manager := events.NewManager()

		ch := handleChannel(ctx, manager, func(*indexerTypes.NewBlock, chan<- int) {
			panic("filter panic")
		})

		// Wait for handleChannel to subscribe, so the event below reaches it.
		synctest.Wait()
		manager.SignalEvent(&indexerTypes.NewBlock{Block: &types.Block{}})

		_, open := <-ch
		require.False(t, open)
		require.Equal(t, "filter panic", recovered)
	})
}
