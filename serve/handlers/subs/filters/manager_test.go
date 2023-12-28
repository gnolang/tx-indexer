package filters

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/serve/handlers/subs/filters/filter"
	"github.com/gnolang/tx-indexer/serve/handlers/subs/filters/mocks"
	"github.com/gnolang/tx-indexer/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// generateTestHashes generates dummy test hashes
func generateTestHashes(t *testing.T, count int) [][]byte {
	t.Helper()

	result := make([][]byte, count)

	for i := 0; i < count; i++ {
		result[i] = []byte(fmt.Sprintf("hash %d", i))
	}

	return result
}

// Test_BlockFilters tests block filters
func Test_BlockFilters(t *testing.T) {
	t.Parallel()

	filterManager := NewFilterManager(
		context.Background(),
		&mocks.MockStorage{},
		events.NewManager(),
		WithLogger(zap.NewNop()),
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
		hashes  = generateTestHashes(t, 10)
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

	for _, hash := range hashes {
		hash := hash

		blockCh <- &types.NewBlock{
			Block: &mocks.MockBlock{
				HashFn: func() []byte {
					return hash
				},
			},
		}
	}

	var (
		wg             sync.WaitGroup
		capturedHashes [][]byte
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
				blockHashesRaw := blockFilter.GetChanges()

				blockHashes, _ := blockHashesRaw.([][]byte)

				if len(blockHashes) == 0 {
					continue
				}

				capturedHashes = blockHashes

				return
			}
		}
	}()

	wg.Wait()

	// Hashes should match
	assert.Equal(t, hashes, capturedHashes)
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
