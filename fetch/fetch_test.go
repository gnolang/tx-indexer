package fetch

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gnolang/gno/tm2/pkg/amino"
	abci "github.com/gnolang/gno/tm2/pkg/bft/abci/types"
	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	"github.com/gnolang/gno/tm2/pkg/bft/state"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/gno/tm2/pkg/std"
	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNodeFetcher_FetchTransactions_Invalid(t *testing.T) {
	t.Parallel()

	t.Run("unable to fetch latest storage transaction", func(t *testing.T) {
		t.Parallel()

		var (
			fetchErr = errors.New("random DB error")

			mockStorage = &mockStorage{
				getLatestSavedHeightFn: func() (int64, error) {
					return 0, fetchErr
				},
			}
		)

		// Create the fetcher
		f := NewFetcher(mockStorage, &mockClient{}, WithLogger(zap.NewNop()))

		assert.ErrorIs(
			t,
			f.FetchTransactions(context.Background()),
			fetchErr,
		)
	})

	t.Run("unable to get latest block height", func(t *testing.T) {
		t.Parallel()

		var (
			fetchErr = errors.New("unable to get block height")

			mockStorage = &mockStorage{
				getLatestSavedHeightFn: func() (int64, error) {
					return 0, nil
				},
			}

			mockClient = &mockClient{
				getLatestBlockNumberFn: func() (int64, error) {
					return 0, fetchErr
				},
			}
		)

		// Create the fetcher
		f := NewFetcher(mockStorage, mockClient)

		assert.ErrorIs(
			t,
			f.FetchTransactions(context.Background()),
			fetchErr,
		)
	})

	t.Run("unable to get block data", func(t *testing.T) {
		t.Parallel()

		var (
			blockNum = int64(10)
			fetchErr = errors.New("unable to get block data")

			mockStorage = &mockStorage{
				getLatestSavedHeightFn: func() (int64, error) {
					return blockNum - 1, nil
				},
			}

			mockClient = &mockClient{
				getLatestBlockNumberFn: func() (int64, error) {
					return blockNum, nil
				},
				getBlockFn: func(num int64) (*core_types.ResultBlock, error) {
					// Sanity check
					require.Equal(t, num, blockNum)

					return nil, fetchErr
				},
			}
		)

		// Create the fetcher
		f := NewFetcher(mockStorage, mockClient)

		assert.ErrorIs(
			t,
			f.FetchTransactions(context.Background()),
			fetchErr,
		)
	})

	t.Run("unable to get block results", func(t *testing.T) {
		t.Parallel()

		var (
			blockNum = int64(10)
			fetchErr = errors.New("unable to get block results")

			mockStorage = &mockStorage{
				getLatestSavedHeightFn: func() (int64, error) {
					return blockNum - 1, nil
				},
			}

			mockClient = &mockClient{
				getLatestBlockNumberFn: func() (int64, error) {
					return blockNum, nil
				},
				getBlockFn: func(num int64) (*core_types.ResultBlock, error) {
					// Sanity check
					require.Equal(t, num, blockNum)

					return &core_types.ResultBlock{
						BlockMeta: nil,
						Block: &types.Block{
							Header: types.Header{
								NumTxs: 1, // > 0
							},
						},
					}, nil
				},
				getBlockResultsFn: func(num int64) (*core_types.ResultBlockResults, error) {
					// Sanity check
					require.Equal(t, num, blockNum)

					return nil, fetchErr
				},
			}
		)

		// Create the fetcher
		f := NewFetcher(mockStorage, mockClient)

		assert.ErrorIs(
			t,
			f.FetchTransactions(context.Background()),
			fetchErr,
		)
	})
}

func TestNodeFetcher_FetchTransactions_Valid(t *testing.T) {
	t.Parallel()

	t.Run("valid txs flow", func(t *testing.T) {
		t.Parallel()

		var cancelFn context.CancelFunc

		var (
			blockNum      = 10
			txCount       = 5
			txs           = generateTransactions(t, txCount)
			serializedTxs = serializeTxs(t, txs)
			blocks        = generateBlocks(t, blockNum+1, txs)

			savedTxs    = make([]*types.TxResult, 0, txCount*blockNum)
			savedBlocks = make([]*types.Block, 0, blockNum)

			mockStorage = &mockStorage{
				getLatestSavedHeightFn: func() (int64, error) {
					return 0, storageErrors.ErrNotFound
				},
				saveBlockFn: func(block *types.Block) error {
					savedBlocks = append(savedBlocks, block)

					return nil
				},
				saveTxFn: func(result *types.TxResult) error {
					savedTxs = append(savedTxs, result)

					return nil
				},
			}

			mockClient = &mockClient{
				getLatestBlockNumberFn: func() (int64, error) {
					return int64(blockNum), nil
				},
				getBlockFn: func(num int64) (*core_types.ResultBlock, error) {
					// Sanity check
					if num > int64(blockNum) {
						t.Fatalf("invalid block requested, %d", num)
					}

					return &core_types.ResultBlock{
						Block: blocks[num],
					}, nil
				},
				getBlockResultsFn: func(num int64) (*core_types.ResultBlockResults, error) {
					// Sanity check
					if num > int64(blockNum) {
						t.Fatalf("invalid block requested, %d", num)
					}

					// Check if all blocks are synced
					if num == int64(blockNum) {
						// At this point, we can cancel the process
						cancelFn()
					}

					return &core_types.ResultBlockResults{
						Height: num,
						Results: &state.ABCIResponses{
							DeliverTxs: make([]abci.ResponseDeliverTx, txCount),
						},
					}, nil
				},
			}
		)

		// Create the fetcher
		f := NewFetcher(mockStorage, mockClient)

		// Create the context
		ctx, cancelFn := context.WithCancel(context.Background())
		defer cancelFn()

		// Run the fetch
		require.NoError(t, f.FetchTransactions(ctx))

		// Verify the transactions are saved correctly
		assert.Len(t, savedTxs, blockNum*txCount)

		for blockIndex := 0; blockIndex < blockNum; blockIndex++ {
			assert.Equal(t, blocks[blockIndex+1], savedBlocks[blockIndex])

			for txIndex := 0; txIndex < txCount; txIndex++ {
				// since this is a linearized array of transactions
				// we can access each item with: blockNum * length + txIndx
				// where blockNum is the y-axis, and txIndx is the x-axis
				tx := savedTxs[blockIndex*txCount+txIndex]

				assert.EqualValues(t, blockIndex+1, tx.Height)
				assert.EqualValues(t, txIndex, tx.Index)
				assert.Equal(t, serializedTxs[txIndex], tx.Tx)
			}
		}
	})

	t.Run("no txs in block", func(t *testing.T) {
		t.Parallel()

		var cancelFn context.CancelFunc

		var (
			blockNum = 5
			blocks   = generateBlocks(t, blockNum+1, []*std.Tx{})

			savedBlocks = make([]*types.Block, 0, blockNum)

			mockStorage = &mockStorage{
				getLatestSavedHeightFn: func() (int64, error) {
					return 0, storageErrors.ErrNotFound
				},
				saveBlockFn: func(block *types.Block) error {
					savedBlocks = append(savedBlocks, block)

					return nil
				},
				saveTxFn: func(_ *types.TxResult) error {
					t.Fatalf("should not save txs")

					return nil
				},
			}

			mockClient = &mockClient{
				getLatestBlockNumberFn: func() (int64, error) {
					return int64(blockNum), nil
				},
				getBlockFn: func(num int64) (*core_types.ResultBlock, error) {
					// Sanity check
					if num > int64(blockNum) {
						t.Fatalf("invalid block requested, %d", num)
					}

					// Check if all blocks are synced
					if num == int64(blockNum) {
						// At this point, we can cancel the process
						cancelFn()
					}

					return &core_types.ResultBlock{
						Block: blocks[num],
					}, nil
				},
				getBlockResultsFn: func(num int64) (*core_types.ResultBlockResults, error) {
					t.Fatalf("should not request results")

					return nil, nil
				},
			}
		)

		// Create the fetcher
		f := NewFetcher(mockStorage, mockClient)

		// Create the context
		ctx, cancelFn := context.WithCancel(context.Background())
		defer cancelFn()

		// Run the fetch
		require.NoError(t, f.FetchTransactions(ctx))

		for blockIndex := 0; blockIndex < blockNum; blockIndex++ {
			assert.Equal(t, blocks[blockIndex+1], savedBlocks[blockIndex])
		}
	})
}

// generateTransactions generates dummy transactions
func generateTransactions(t *testing.T, count int) []*std.Tx {
	t.Helper()

	txs := make([]*std.Tx, count)

	for i := 0; i < count; i++ {
		txs[i] = &std.Tx{
			Memo: fmt.Sprintf("memo %d", i),
		}
	}

	return txs
}

// generateBlocks generates dummy blocks
func generateBlocks(
	t *testing.T,
	count int,
	txs []*std.Tx,
) []*types.Block {
	t.Helper()

	blocks := make([]*types.Block, count)

	for i := 0; i < count; i++ {
		blocks[i] = &types.Block{
			Header: types.Header{
				NumTxs: int64(len(txs)),
				Height: int64(i),
			},
			Data: types.Data{
				Txs: serializeTxs(t, txs),
			},
		}
	}

	return blocks
}

// serializeTxs encodes the transactions into Amino JSON
func serializeTxs(t *testing.T, txs []*std.Tx) types.Txs {
	t.Helper()

	serializedTxs := make(types.Txs, 0, len(txs))

	for _, tx := range txs {
		serializedTx, err := amino.MarshalJSON(tx)
		require.NoError(t, err)

		serializedTxs = append(serializedTxs, serializedTx)
	}

	return serializedTxs
}
