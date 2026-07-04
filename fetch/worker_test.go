package fetch

import (
	"context"
	"errors"
	"sync"
	"testing"

	abci "github.com/gnolang/gno/tm2/pkg/bft/abci/types"
	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	"github.com/gnolang/gno/tm2/pkg/bft/state"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockBlockResults builds a block results response with numTxs deliver results.
func mockBlockResults(height int64, numTxs int) *core_types.ResultBlockResults {
	return &core_types.ResultBlockResults{
		Height: height,
		Results: &state.ABCIResponses{
			DeliverTxs: make([]abci.ResponseDeliverTx, numTxs),
		},
	}
}

// blockHeightsOf returns the heights of the given blocks, in order.
func blockHeightsOf(blocks []*types.Block) []int64 {
	heights := make([]int64, 0, len(blocks))
	for _, block := range blocks {
		heights = append(heights, block.Height)
	}

	return heights
}

func TestFetchBlocksWithRetry(t *testing.T) {
	t.Parallel()

	txs := generateTransactions(t, 0) // empty blocks keep this focused on block fetching
	blocks := generateBlocks(t, 6, txs)

	t.Run("keeps successful blocks and retries only the failed height", func(t *testing.T) {
		t.Parallel()

		var (
			mu    sync.Mutex
			calls = make(map[uint64]int)
		)

		client := &mockClient{
			createBatchFn: erroringBatch,
			getBlockFn: func(num uint64) (*core_types.ResultBlock, error) {
				mu.Lock()
				calls[num]++
				attempt := calls[num]
				mu.Unlock()

				// Height 3 fails on its first two attempts, then succeeds
				if num == 3 && attempt < 3 {
					return nil, errors.New("flaky 502")
				}

				return &core_types.ResultBlock{Block: blocks[num]}, nil
			},
		}

		result := fetchBlocksWithRetry(context.Background(), client, chunkRange{from: 1, to: 5}, fastRetry, zap.NewNop())

		require.Equal(t, []int64{1, 2, 3, 4, 5}, blockHeightsOf(result))

		// Only the failing height was retried; the rest were fetched exactly once
		mu.Lock()
		defer mu.Unlock()

		assert.Equal(t, 3, calls[3])

		for _, h := range []uint64{1, 2, 4, 5} {
			assert.Equal(t, 1, calls[h], "height %d should not be retried", h)
		}
	})

	t.Run("returns a partial set when a height never succeeds", func(t *testing.T) {
		t.Parallel()

		client := &mockClient{
			createBatchFn: erroringBatch,
			getBlockFn: func(num uint64) (*core_types.ResultBlock, error) {
				if num == 4 {
					return nil, errors.New("permanently gone")
				}

				return &core_types.ResultBlock{Block: blocks[num]}, nil
			},
		}

		result := fetchBlocksWithRetry(context.Background(), client, chunkRange{from: 1, to: 5}, fastRetry, zap.NewNop())

		// Height 4 is absent, the rest are present and sorted
		assert.Equal(t, []int64{1, 2, 3, 5}, blockHeightsOf(result))
	})
}

func TestFetchChunk(t *testing.T) {
	t.Parallel()

	const txCount = 1

	txs := generateTransactions(t, txCount)
	blocks := generateBlocks(t, 6, txs)

	// alwaysBlocks serves every block in the range successfully. The error
	// return is required by getBlockDelegate even though it is always nil.
	alwaysBlocks := func(num uint64) (*core_types.ResultBlock, error) { //nolint:unparam // delegate signature
		return &core_types.ResultBlock{Block: blocks[num]}, nil
	}

	testTable := []struct {
		name            string
		buildClient     func() *mockClient
		expectedHeights []int64
		expectedMissing []uint64
	}{
		{
			"all blocks and results fetched",
			func() *mockClient {
				return &mockClient{
					createBatchFn: erroringBatch,
					getBlockFn:    alwaysBlocks,
					getBlockResultsFn: func(num uint64) (*core_types.ResultBlockResults, error) {
						return mockBlockResults(int64(num), txCount), nil
					},
				}
			},
			[]int64{2, 3, 4, 5},
			nil,
		},
		{
			"block that never fetches is reported missing",
			func() *mockClient {
				return &mockClient{
					createBatchFn: erroringBatch,
					getBlockFn: func(num uint64) (*core_types.ResultBlock, error) {
						if num == 4 {
							return nil, errors.New("permanently gone")
						}

						return &core_types.ResultBlock{Block: blocks[num]}, nil
					},
					getBlockResultsFn: func(num uint64) (*core_types.ResultBlockResults, error) {
						return mockBlockResults(int64(num), txCount), nil
					},
				}
			},
			[]int64{2, 3, 5},
			[]uint64{4},
		},
		{
			"tx results are retried and then succeed",
			func() *mockClient {
				var (
					mu    sync.Mutex
					calls = make(map[uint64]int)
				)

				return &mockClient{
					createBatchFn: erroringBatch,
					getBlockFn:    alwaysBlocks,
					getBlockResultsFn: func(num uint64) (*core_types.ResultBlockResults, error) {
						mu.Lock()
						calls[num]++
						attempt := calls[num]
						mu.Unlock()

						// Height 4 results fail once, then succeed
						if num == 4 && attempt < 2 {
							return nil, errors.New("flaky results")
						}

						return mockBlockResults(int64(num), txCount), nil
					},
				}
			},
			[]int64{2, 3, 4, 5},
			nil,
		},
		{
			"block whose results never fetch is dropped and reported missing",
			func() *mockClient {
				return &mockClient{
					createBatchFn: erroringBatch,
					getBlockFn:    alwaysBlocks,
					getBlockResultsFn: func(num uint64) (*core_types.ResultBlockResults, error) {
						if num == 4 {
							return nil, errors.New("results permanently gone")
						}

						return mockBlockResults(int64(num), txCount), nil
					},
				}
			},
			[]int64{2, 3, 5},
			[]uint64{4},
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			c, missing, err := fetchChunk(
				context.Background(),
				testCase.buildClient(),
				chunkRange{from: 2, to: 5},
				fastRetry,
				zap.NewNop(),
			)

			assert.Equal(t, testCase.expectedHeights, blockHeightsOf(c.blocks))
			assert.Equal(t, testCase.expectedMissing, missing)

			if len(testCase.expectedMissing) > 0 {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBuildTxResults(t *testing.T) {
	t.Parallel()

	txCount := 3
	txs := generateTransactions(t, txCount)
	block := generateBlocks(t, 2, txs)[1] // block with txCount txs

	t.Run("nil results", func(t *testing.T) {
		t.Parallel()

		_, err := buildTxResults(block, nil)
		assert.Error(t, err)
	})

	t.Run("fewer deliver results than txs", func(t *testing.T) {
		t.Parallel()

		_, err := buildTxResults(block, mockBlockResults(block.Height, txCount-1))
		assert.Error(t, err)
	})

	t.Run("builds one result per tx", func(t *testing.T) {
		t.Parallel()

		results, err := buildTxResults(block, mockBlockResults(block.Height, txCount))
		require.NoError(t, err)
		require.Len(t, results, txCount)

		for index, result := range results {
			assert.Equal(t, block.Height, result.Height)
			assert.EqualValues(t, index, result.Index)
			assert.Equal(t, block.Txs[index], result.Tx)
		}
	})
}
