package fetch

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	clientTypes "github.com/gnolang/tx-indexer/client/types"
	"github.com/gnolang/tx-indexer/internal/metricstest"
	"github.com/gnolang/tx-indexer/metrics"
	"github.com/gnolang/tx-indexer/storage"
)

// unbatchedChain is a node whose batches fail, so the fetcher reads each block
// alone with block; height answers the chain's latest height. Its blocks carry
// no transaction.
func unbatchedChain(
	height func() (uint64, error),
	block func(uint64) (*core_types.ResultBlock, error),
) *mockClient {
	return &mockClient{
		createBatchFn: func() clientTypes.Batch {
			return &mockBatch{
				executeFn: func(context.Context) ([]any, error) {
					return nil, errors.New("batch unavailable")
				},
			}
		},
		getLatestBlockNumberFn: height,
		getBlockFn:             block,
	}
}

// emptyBlock is block number, holding no transaction
func emptyBlock(number uint64) (*core_types.ResultBlock, error) {
	return &core_types.ResultBlock{
		Block: &types.Block{Header: types.Header{Height: int64(number)}},
	}, nil
}

// newStore opens a fresh database for the test
func newStore(t *testing.T) *storage.Pebble {
	t.Helper()

	s, err := storage.NewPebble(t.TempDir())
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, s.Close())
	})

	return s
}

// fetchUntil runs a fetcher of client into s, recording into m and polling
// the chain every interval, until reached holds
func fetchUntil(
	t *testing.T,
	s storage.Storage,
	client Client,
	m *metrics.Metrics,
	interval time.Duration,
	reached func(c *assert.CollectT),
) {
	t.Helper()

	f := New(s, client, &mockEvents{}, WithMetrics(m))
	f.queryInterval = interval

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan error, 1)

	go func() {
		stopped <- f.FetchChainData(ctx)
	}()

	assert.EventuallyWithT(t, reached, 5*time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, <-stopped)
}

// indexed holds once m reports height indexed
func indexed(m *metrics.Metrics, height float64) func(c *assert.CollectT) {
	return func(c *assert.CollectT) {
		assert.InDelta(c, height, metricstest.Value(c, m, "tx_indexer_indexed_height", nil), 0)
	}
}

// The fetcher reports the chain's height as it reads it from the node, and
// the height it has indexed up to as soon as it writes: its first poll of the
// chain is its only one here, so the write itself reports the new height
func TestFetcher_ReportsTheHeights(t *testing.T) {
	t.Parallel()

	m := metrics.New()

	chain := unbatchedChain(func() (uint64, error) { return 10, nil }, emptyBlock)

	fetchUntil(t, newStore(t), chain, m, time.Hour, indexed(m, 10))

	assert.InDelta(t, 10, metricstest.Value(t, m, "tx_indexer_chain_height", nil), 0)
}

// A fetcher started on a database already indexed up to the chain reports the
// height it holds, though it has nothing to write
func TestFetcher_ReportsTheHeightItStartsFrom(t *testing.T) {
	t.Parallel()

	s := newStore(t)

	wb := s.WriteBatch()
	require.NoError(t, wb.SetLatestHeight(7))
	require.NoError(t, wb.Commit())

	m := metrics.New()

	chain := unbatchedChain(
		func() (uint64, error) { return 7, nil },
		func(number uint64) (*core_types.ResultBlock, error) {
			return nil, fmt.Errorf("block %d is indexed already", number)
		},
	)

	fetchUntil(t, s, chain, m, time.Hour, indexed(m, 7))
}

// A failed read of the chain's height and a failed range fetch each count as
// a fetch error of their stage, and the range fetched again as a retry
func TestFetcher_CountsFetchErrorsAndRetries(t *testing.T) {
	t.Parallel()

	m := metrics.New()

	var heightReads, firstBlockReads atomic.Int32

	chain := unbatchedChain(
		func() (uint64, error) {
			if heightReads.Add(1) == 1 {
				return 0, errors.New("node unreachable")
			}

			return 5, nil
		},
		func(number uint64) (*core_types.ResultBlock, error) {
			if number == 1 && firstBlockReads.Add(1) == 1 {
				return nil, errors.New("block unavailable")
			}

			return emptyBlock(number)
		},
	)

	// A short poll, so the failed range is fetched again soon
	fetchUntil(t, newStore(t), chain, m, 10*time.Millisecond, indexed(m, 5))

	chainHeightErrors := map[string]string{"stage": "chain_height"}
	rangeErrors := map[string]string{"stage": "range"}

	assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_fetch_errors_total", chainHeightErrors), 0)
	assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_fetch_errors_total", rangeErrors), 0)
	assert.InDelta(t, 1, metricstest.Value(t, m, "tx_indexer_fetch_retries_total", nil), 0)
}
