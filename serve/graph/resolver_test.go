package graph

import (
	"testing"
	"time"

	"github.com/99designs/gqlgen/client"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/gnolang/tx-indexer/events"
	indexerTypes "github.com/gnolang/tx-indexer/types"
)

func TestSubscription_FilterPanicEndsOnlyItsSubscription(t *testing.T) {
	t.Parallel()

	const height = 7

	manager := events.NewManager()
	c := client.New(Setup(nil, manager, nil, chi.NewMux(), true), client.Path("/graphql/query"))

	// Subscriptions register with the manager asynchronously, so keep signalling until the test ends.
	ctx := t.Context()

	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				manager.SignalEvent(&indexerTypes.NewBlock{Block: &types.Block{Header: types.Header{Height: height}}})
			}
		}
	}()

	var resp struct {
		GetBlocks struct {
			Height int64 `json:"height"`
		} `json:"getBlocks"`
	}

	// A null entry in `_and` makes the generated filter call Eval on a nil receiver.
	panicking := c.Websocket(`subscription { getBlocks(where: {_and: [null]}) { height } }`)

	t.Cleanup(func() { require.NoError(t, panicking.Close()) })

	require.ErrorContains(t, panicking.Next(&resp), `"complete"`)

	healthy := c.Websocket(`subscription { getBlocks(where: {}) { height } }`)

	t.Cleanup(func() { require.NoError(t, healthy.Close()) })

	require.NoError(t, healthy.Next(&resp))
	require.EqualValues(t, height, resp.GetBlocks.Height)
}
