package filters

import (
	"context"
	"sync"
	"testing"
	"time"

	tm2Types "github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/serve/filters/filter"
	"github.com/gnolang/tx-indexer/serve/filters/mocks"
	"github.com/gnolang/tx-indexer/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateBlocks generates dummy blocks
func generateBlocks(t *testing.T, count int) []*tm2Types.Block {
	t.Helper()

	blocks := make([]*tm2Types.Block, count)

	for i := 0; i < count; i++ {
		blocks[i] = &tm2Types.Block{
			Header: tm2Types.Header{
				Height: int64(i),
			},
			Data: tm2Types.Data{},
		}
	}

	return blocks
}

// Test_BlockFilters tests block filters
func Test_BlockFilters(t *testing.T) {
	t.Parallel()

	filterManager := NewFilterManager(
		context.Background(),
		&mocks.MockStorage{},
		events.NewManager(),
	)

	// Create block filter
	blockFilterID := filterManager.NewBlockFilter()

	// Fetch the filter
	blockFilter, err := filterManager.GetFilter(blockFilterID)
	require.NoError(t, err)

	// Validate filter type
	require.Equal(t, filter.BlockFilterType, blockFilter.GetType())

	// Get last used
	lastUsed := blockFilter.GetLastUsed()

	// Update last used
	blockFilter.UpdateLastUsed()

	// Check if last used changed
	require.True(t, blockFilter.GetLastUsed().After(lastUsed))

	// Check if filter exists and remove it
	require.True(t, filterManager.UninstallFilter(blockFilterID))

	// Filter should not exist anymore
	require.False(t, filterManager.UninstallFilter(blockFilterID))
}

// Test_NewBlockEvents tests subscribing to new block events
func Test_NewBlockEvents(t *testing.T) {
	t.Parallel()

	var (
		blocks  = generateBlocks(t, 10)
		blockCh = make(chan events.Event)

		mockEvents = &mocks.MockEvents{
			SubscribeFn: func(_ []events.Type) *events.Subscription {
				return &events.Subscription{
					SubCh: blockCh,
				}
			},
		}
	)

	// Init filter manager
	filterManager := NewFilterManager(
		context.Background(),
		&mocks.MockStorage{},
		mockEvents,
	)

	// Create block filter
	id := filterManager.NewBlockFilter()
	defer filterManager.UninstallFilter(id)

	for _, block := range blocks {
		block := block

		blockCh <- &types.NewBlock{
			Block: block,
		}
	}

	var (
		wg              sync.WaitGroup
		capturedHeaders []tm2Types.Header
	)

	wg.Add(1)

	go func() {
		deadline := time.After(5 * time.Second)
		ticker := time.NewTicker(100 * time.Millisecond)

		defer func() {
			ticker.Stop()
			wg.Done()
		}()

		for {
			select {
			case <-deadline:
				return
			case <-ticker.C:
				// Get filter
				blockFilter, err := filterManager.GetFilter(id)
				require.Nil(t, err)

				// Get changes
				blockHeadersRaw := blockFilter.GetChanges()

				blockHeaders, ok := blockHeadersRaw.([]tm2Types.Header)
				require.True(t, ok)

				if len(blockHeaders) == 0 {
					continue
				}

				capturedHeaders = blockHeaders

				return
			}
		}
	}()

	wg.Wait()

	// Make sure the headers match
	require.Len(t, capturedHeaders, len(blocks))

	for index, header := range capturedHeaders {
		assert.Equal(t, blocks[index].Header, header)
	}
}

func Test_FilterCleanup(t *testing.T) {
	t.Parallel()

	// Create filter manager
	filterManager := NewFilterManager(
		context.Background(),
		&mocks.MockStorage{},
		events.NewManager(),
		WithCleanupInterval(10*time.Millisecond),
	)

	// Create block filter
	id := filterManager.NewBlockFilter()

	var (
		capturedErr error
		wg          sync.WaitGroup
	)

	wg.Add(1)

	go func() {
		deadline := time.After(5 * time.Second)
		ticker := time.NewTicker(100 * time.Millisecond)

		defer func() {
			ticker.Stop()
			wg.Done()
		}()

		for {
			select {
			case <-deadline:
				return
			case <-ticker.C:
				if _, err := filterManager.GetFilter(id); err != nil {
					capturedErr = err

					return
				}
			}
		}
	}()

	wg.Wait()

	require.Error(t, capturedErr)
}
