package fetch

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gnolang/gno/gno.land/pkg/gnoland"
	"github.com/gnolang/gno/tm2/pkg/amino"
	abci "github.com/gnolang/gno/tm2/pkg/bft/abci/types"
	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	"github.com/gnolang/gno/tm2/pkg/bft/state"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/gno/tm2/pkg/std"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	clientTypes "github.com/gnolang/tx-indexer/client/types"
	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/internal/mock"
	"github.com/gnolang/tx-indexer/storage"
	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
	indexerTypes "github.com/gnolang/tx-indexer/types"
)

func TestFetcher_FetchTransactions_Invalid(t *testing.T) {
	t.Parallel()

	t.Run("unable to fetch latest storage transaction", func(t *testing.T) {
		t.Parallel()

		var (
			fetchErr = errors.New("random DB error")

			mockStorage = &mock.Storage{
				GetLatestSavedHeightFn: func() (uint64, error) {
					return 0, fetchErr
				},
			}
		)

		// Create the fetcher
		f := New(
			mockStorage,
			&mockClient{},
			&mockEvents{},
			WithLogger(zap.NewNop()),
		)

		assert.ErrorIs(
			t,
			f.FetchChainData(context.Background()),
			fetchErr,
		)
	})
}

func TestFetcher_FetchTransactions_Valid_FullBlocks(t *testing.T) {
	t.Parallel()

	t.Run("valid txs flow, sequential", func(t *testing.T) {
		t.Parallel()

		var cancelFn context.CancelFunc

		var (
			blockNum      = 1000
			txCount       = 10
			txs           = generateTransactions(t, txCount)
			serializedTxs = serializeTxs(t, txs)
			blocks        = generateBlocks(t, blockNum+1, txs)

			savedTxs       = make([]*types.TxResult, 0, txCount*blockNum)
			savedBlocks    = make([]*types.Block, 0, blockNum)
			capturedEvents = make([]events.Event, 0)

			mockEvents = &mockEvents{
				signalEventFn: func(e events.Event) {
					if e.GetType() == indexerTypes.NewBlockEvent {
						_, ok := e.(*indexerTypes.NewBlock)
						require.True(t, ok)
						capturedEvents = append(capturedEvents, e)
					}
				},
			}

			latestSaved = uint64(0)

			mockStorage = &mock.Storage{
				GetLatestSavedHeightFn: func() (uint64, error) {
					if latestSaved == 0 {
						return 0, storageErrors.ErrNotFound
					}

					return latestSaved, nil
				},
				GetWriteBatchFn: func() storage.Batch {
					return &mock.WriteBatch{
						SetBlockFn: func(block *types.Block) error {
							savedBlocks = append(savedBlocks, block)

							// Check if all blocks are saved
							if block.Height == int64(blockNum) {
								// At this point, we can cancel the process
								cancelFn()
							}

							latestSaved = uint64(block.Height)

							return nil
						},
						SetTxFn: func(result *types.TxResult) error {
							savedTxs = append(savedTxs, result)

							return nil
						},
					}
				},
			}

			mockClient = &mockClient{
				createBatchFn: func() clientTypes.Batch {
					return &mockBatch{
						executeFn: func(_ context.Context) ([]any, error) {
							// Force an error
							return nil, errors.New("something is flaky")
						},
						countFn: func() int {
							return 1 // to trigger execution
						},
					}
				},
				getLatestBlockNumberFn: func() (uint64, error) {
					return uint64(blockNum), nil
				},
				getBlockFn: func(num uint64) (*core_types.ResultBlock, error) {
					// Sanity check
					if num > uint64(blockNum) {
						t.Fatalf("invalid block requested, %d", num)
					}

					return &core_types.ResultBlock{
						Block: blocks[num],
					}, nil
				},
				getBlockResultsFn: func(num uint64) (*core_types.ResultBlockResults, error) {
					// Sanity check
					if num > uint64(blockNum) {
						t.Fatalf("invalid block requested, %d", num)
					}

					return &core_types.ResultBlockResults{
						Height: int64(num),
						Results: &state.ABCIResponses{
							DeliverTxs: make([]abci.ResponseDeliverTx, txCount),
						},
					}, nil
				},
				getGenesisFn: func() (*core_types.ResultGenesis, error) {
					return &core_types.ResultGenesis{
						Genesis: &types.GenesisDoc{
							AppState: gnoland.GnoGenesisState{
								Balances: []gnoland.Balance{},
								Txs:      []gnoland.TxWithMetadata{},
							},
						},
					}, nil
				},
			}
		)

		// Create the fetcher
		f := New(
			mockStorage,
			mockClient,
			mockEvents,
			WithMaxSlots(10),
			WithMaxChunkSize(50),
		)

		// Short interval to force spawning
		f.queryInterval = 100 * time.Millisecond

		// Create the context
		ctx, cancelFn := context.WithCancel(context.Background())
		defer cancelFn()

		// Run the fetch
		require.NoError(t, f.FetchChainData(ctx))

		// Verify the transactions are saved correctly
		require.Len(t, savedTxs, blockNum*txCount)

		for blockIndex := 1; blockIndex < blockNum; blockIndex++ {
			assert.Equal(t, blocks[blockIndex], savedBlocks[blockIndex])

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

		// Make sure proper events were emitted
		require.Len(t, capturedEvents, len(blocks))

		for index, event := range capturedEvents {
			if index == 0 {
				// Dummy genesis block
				continue
			}

			if event.GetType() != indexerTypes.NewBlockEvent {
				continue
			}

			eventData, ok := event.(*indexerTypes.NewBlock)
			require.True(t, ok)

			// Make sure the block is valid
			assert.Equal(t, blocks[index], eventData.Block)

			// Make sure the transaction results are valid
			require.Len(t, eventData.Results, txCount)

			for txIndex, tx := range eventData.Results {
				assert.EqualValues(t, blocks[index].Height, tx.Height)
				assert.EqualValues(t, txIndex, tx.Index)
				assert.Equal(t, serializedTxs[txIndex], tx.Tx)
			}
		}
	})

	t.Run("valid txs flow, batch", func(t *testing.T) {
		t.Parallel()

		var cancelFn context.CancelFunc

		var (
			blockNum      = 1000
			txCount       = 10
			txs           = generateTransactions(t, txCount)
			serializedTxs = serializeTxs(t, txs)
			blocks        = generateBlocks(t, blockNum+1, txs)

			savedTxs       = make([]*types.TxResult, 0, txCount*blockNum)
			savedBlocks    = make([]*types.Block, 0, blockNum)
			capturedEvents = make([]events.Event, 0)

			mockEvents = &mockEvents{
				signalEventFn: func(e events.Event) {
					if e.GetType() == indexerTypes.NewBlockEvent {
						_, ok := e.(*indexerTypes.NewBlock)
						require.True(t, ok)
						capturedEvents = append(capturedEvents, e)
					}
				},
			}

			latestSaved = uint64(0)

			mockStorage = &mock.Storage{
				GetLatestSavedHeightFn: func() (uint64, error) {
					if latestSaved == 0 {
						return 0, storageErrors.ErrNotFound
					}

					return latestSaved, nil
				},
				GetWriteBatchFn: func() storage.Batch {
					return &mock.WriteBatch{
						SetBlockFn: func(block *types.Block) error {
							savedBlocks = append(savedBlocks, block)

							// Check if all blocks are saved
							if block.Height == int64(blockNum) {
								// At this point, we can cancel the process
								cancelFn()
							}

							latestSaved = uint64(block.Height)

							return nil
						},
						SetTxFn: func(result *types.TxResult) error {
							savedTxs = append(savedTxs, result)

							return nil
						},
					}
				},
			}

			batch = make([]any, 0)

			mockClient = &mockClient{
				createBatchFn: func() clientTypes.Batch {
					return &mockBatch{
						executeFn: func(_ context.Context) ([]any, error) {
							results := make([]any, len(batch))
							copy(results, batch)

							batch = batch[:0]

							return results, nil
						},
						countFn: func() int {
							return len(batch)
						},
						addBlockRequestFn: func(num uint64) error {
							// Sanity check
							if num > uint64(blockNum) {
								t.Fatalf("invalid block requested, %d", num)
							}

							batch = append(
								batch,
								&core_types.ResultBlock{
									Block: blocks[num],
								},
							)

							return nil
						},
						addBlockResultsRequestFn: func(num uint64) error {
							// Sanity check
							if num > uint64(blockNum) {
								t.Fatalf("invalid block requested, %d", num)
							}

							batch = append(batch,
								&core_types.ResultBlockResults{
									Height: int64(num),
									Results: &state.ABCIResponses{
										DeliverTxs: make([]abci.ResponseDeliverTx, txCount),
									},
								},
							)

							return nil
						},
					}
				},
				getLatestBlockNumberFn: func() (uint64, error) {
					return uint64(blockNum), nil
				},
				getGenesisFn: func() (*core_types.ResultGenesis, error) {
					return &core_types.ResultGenesis{
						Genesis: &types.GenesisDoc{
							AppState: gnoland.GnoGenesisState{
								Balances: []gnoland.Balance{},
								Txs:      []gnoland.TxWithMetadata{},
							},
						},
					}, nil
				},
				getBlockResultsFn: func(num uint64) (*core_types.ResultBlockResults, error) {
					// Sanity check
					if num > uint64(blockNum) {
						t.Fatalf("invalid block requested, %d", num)
					}

					return &core_types.ResultBlockResults{
						Height: int64(num),
						Results: &state.ABCIResponses{
							DeliverTxs: make([]abci.ResponseDeliverTx, txCount),
						},
					}, nil
				},
			}
		)

		// Create the fetcher
		f := New(
			mockStorage,
			mockClient,
			mockEvents,
			// The reason for limiting this to 1 worker
			// is that the batch is localized in this context
			// and should not be shared between threads. An alternative
			// would be to implement a batch that is unique for each thread
			// (like in the real world). For the sake of simplicity and this test,
			// this is avoided
			WithMaxSlots(1),
			WithMaxChunkSize(500),
		)

		// Short interval to force spawning
		f.queryInterval = 100 * time.Millisecond

		// Create the context
		ctx, cancelFn := context.WithCancel(context.Background())
		defer cancelFn()

		// Run the fetch
		require.NoError(t, f.FetchChainData(ctx))

		// Verify the transactions are saved correctly
		require.Len(t, savedTxs, blockNum*txCount)

		for blockIndex := 1; blockIndex < blockNum; blockIndex++ {
			assert.Equal(t, blocks[blockIndex], savedBlocks[blockIndex])

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

		// Make sure proper events were emitted
		require.Len(t, capturedEvents, len(blocks))

		for index, event := range capturedEvents {
			if index == 0 {
				// Dummy genesis block
				continue
			}

			// Make sure the block is valid
			eventData := event.(*indexerTypes.NewBlock)
			assert.Equal(t, blocks[index], eventData.Block)

			// Make sure the transaction results are valid
			require.Len(t, eventData.Results, txCount)

			for txIndex, tx := range eventData.Results {
				assert.EqualValues(t, blocks[index].Height, tx.Height)
				assert.EqualValues(t, txIndex, tx.Index)
				assert.Equal(t, serializedTxs[txIndex], tx.Tx)
			}
		}
	})
}

func TestFetcher_FetchTransactions_Valid_FullTransactions(t *testing.T) {
	t.Parallel()

	t.Run("valid txs flow, sequential", func(t *testing.T) {
		t.Parallel()

		var cancelFn context.CancelFunc

		var (
			blockNum      = 1000
			txCount       = 10
			txs           = generateTransactions(t, txCount)
			serializedTxs = serializeTxs(t, txs)
			blocks        = generateBlocks(t, blockNum+1, txs)

			savedTxs       = make([]*types.TxResult, 0, txCount*blockNum)
			savedBlocks    = make([]*types.Block, 0, blockNum)
			capturedEvents = make([]events.Event, 0)

			mockEvents = &mockEvents{
				signalEventFn: func(e events.Event) {
					if e.GetType() == indexerTypes.NewBlockEvent {
						_, ok := e.(*indexerTypes.NewBlock)
						require.True(t, ok)
						capturedEvents = append(capturedEvents, e)
					}
				},
			}

			latestSaved = uint64(0)

			mockStorage = &mock.Storage{
				GetLatestSavedHeightFn: func() (uint64, error) {
					if latestSaved == 0 {
						return 0, storageErrors.ErrNotFound
					}

					return latestSaved, nil
				},
				GetWriteBatchFn: func() storage.Batch {
					return &mock.WriteBatch{
						SetBlockFn: func(block *types.Block) error {
							savedBlocks = append(savedBlocks, block)

							// Check if all blocks are saved
							if block.Height == int64(blockNum) {
								// At this point, we can cancel the process
								cancelFn()
							}

							latestSaved = uint64(block.Height)

							return nil
						},
						SetTxFn: func(result *types.TxResult) error {
							savedTxs = append(savedTxs, result)

							return nil
						},
					}
				},
			}

			mockClient = &mockClient{
				createBatchFn: func() clientTypes.Batch {
					return &mockBatch{
						executeFn: func(_ context.Context) ([]any, error) {
							// Force an error
							return nil, errors.New("something is flaky")
						},
						countFn: func() int {
							return 1 // to trigger execution
						},
					}
				},
				getLatestBlockNumberFn: func() (uint64, error) {
					return uint64(blockNum), nil
				},
				getBlockFn: func(num uint64) (*core_types.ResultBlock, error) {
					// Sanity check
					if num > uint64(blockNum) {
						t.Fatalf("invalid block requested, %d", num)
					}

					if len(blocks[num].Txs) != txCount {
						t.Fatalf("invalid transactions, current size: %d", len(blocks[num].Txs))
					}

					return &core_types.ResultBlock{
						Block: blocks[num],
					}, nil
				},
				getBlockResultsFn: func(num uint64) (*core_types.ResultBlockResults, error) {
					// Sanity check
					if num > uint64(blockNum) {
						t.Fatalf("invalid block requested, %d", num)
					}

					return &core_types.ResultBlockResults{
						Height: int64(num),
						Results: &state.ABCIResponses{
							DeliverTxs: make([]abci.ResponseDeliverTx, txCount),
						},
					}, nil
				},
				getGenesisFn: func() (*core_types.ResultGenesis, error) {
					return &core_types.ResultGenesis{
						Genesis: &types.GenesisDoc{
							AppState: gnoland.GnoGenesisState{
								Balances: []gnoland.Balance{},
								Txs:      []gnoland.TxWithMetadata{},
							},
						},
					}, nil
				},
			}
		)

		// Create the fetcher
		f := New(
			mockStorage,
			mockClient,
			mockEvents,
			WithMaxSlots(10),
			WithMaxChunkSize(50),
		)

		// Short interval to force spawning
		f.queryInterval = 100 * time.Millisecond

		// Create the context
		ctx, cancelFn := context.WithCancel(context.Background())
		defer cancelFn()

		// Run the fetch
		require.NoError(t, f.FetchChainData(ctx))

		// Verify the transactions are saved correctly
		require.Len(t, savedTxs, blockNum*txCount)

		for blockIndex := 1; blockIndex < blockNum; blockIndex++ {
			assert.Equal(t, blocks[blockIndex], savedBlocks[blockIndex])

			for txIndex := 0; txIndex < txCount; txIndex++ {
				// since this is a linearized array of transactions
				// we can access each item with: blockNum * length + txIndx
				// where blockNum is the y-axis, and txIndx is the x-axis
				tx := savedTxs[(blockIndex-1)*txCount+txIndex]

				assert.EqualValues(t, blockIndex, tx.Height)
				assert.EqualValues(t, txIndex, tx.Index)
				assert.Equal(t, serializedTxs[txIndex], tx.Tx)
			}
		}

		// Make sure proper events were emitted
		// Blocks each have as many transactions as txCount.
		txEventCount := len(blocks)
		require.Len(t, capturedEvents, txEventCount)

		for index, event := range capturedEvents {
			if index == 0 {
				// Dummy genesis block
				continue
			}

			if event.GetType() != indexerTypes.NewBlockEvent {
				continue
			}

			eventData, ok := event.(*indexerTypes.NewBlock)
			require.True(t, ok)

			// Make sure the block is valid
			assert.Equal(t, blocks[index], eventData.Block)

			// Make sure the transaction results are valid
			require.Len(t, eventData.Results, txCount)

			for txIndex, tx := range eventData.Results {
				assert.EqualValues(t, blocks[index].Height, tx.Height)
				assert.EqualValues(t, txIndex, tx.Index)
				assert.Equal(t, serializedTxs[txIndex], tx.Tx)
			}
		}
	})
}

func TestFetcher_FetchTransactions_Valid_EmptyBlocks(t *testing.T) {
	t.Parallel()

	t.Run("no txs in block, sequential", func(t *testing.T) {
		t.Parallel()

		var cancelFn context.CancelFunc

		var (
			blockNum = 5
			blocks   = generateBlocks(t, blockNum+1, []*std.Tx{})

			savedBlocks    = make([]*types.Block, 0, blockNum)
			capturedEvents = make([]*indexerTypes.NewBlock, 0)

			mockEvents = &mockEvents{
				signalEventFn: func(e events.Event) {
					blockEvent, ok := e.(*indexerTypes.NewBlock)
					require.True(t, ok)

					capturedEvents = append(capturedEvents, blockEvent)
				},
			}

			mockStorage = &mock.Storage{
				GetLatestSavedHeightFn: func() (uint64, error) {
					return 0, storageErrors.ErrNotFound
				},
				GetWriteBatchFn: func() storage.Batch {
					return &mock.WriteBatch{
						SetBlockFn: func(block *types.Block) error {
							savedBlocks = append(savedBlocks, block)

							// Check if all blocks are saved
							if block.Height == int64(blockNum) {
								// At this point, we can cancel the process
								cancelFn()
							}

							return nil
						},
						SetTxFn: func(_ *types.TxResult) error {
							t.Fatalf("should not save txs")

							return nil
						},
					}
				},
			}

			mockClient = &mockClient{
				createBatchFn: func() clientTypes.Batch {
					return &mockBatch{
						executeFn: func(_ context.Context) ([]any, error) {
							// Force an error
							return nil, errors.New("something is flaky")
						},
						countFn: func() int {
							return 1 // to trigger execution
						},
					}
				},
				getLatestBlockNumberFn: func() (uint64, error) {
					return uint64(blockNum), nil
				},
				getBlockFn: func(num uint64) (*core_types.ResultBlock, error) {
					// Sanity check
					if num > uint64(blockNum) {
						t.Fatalf("invalid block requested, %d", num)
					}

					return &core_types.ResultBlock{
						Block: blocks[num],
					}, nil
				},
				getBlockResultsFn: func(num uint64) (*core_types.ResultBlockResults, error) {
					if num == 0 {
						return &core_types.ResultBlockResults{
							Height: int64(num),
							Results: &state.ABCIResponses{
								DeliverTxs: make([]abci.ResponseDeliverTx, 0),
							},
						}, nil
					}

					t.Fatalf("should not request results")

					return nil, nil
				},
				getGenesisFn: func() (*core_types.ResultGenesis, error) {
					return &core_types.ResultGenesis{
						Genesis: &types.GenesisDoc{
							AppState: gnoland.GnoGenesisState{
								Balances: []gnoland.Balance{},
								Txs:      []gnoland.TxWithMetadata{},
							},
						},
					}, nil
				},
			}
		)

		// Create the fetcher
		f := New(mockStorage, mockClient, mockEvents)

		// Create the context
		ctx, cancelFn := context.WithCancel(context.Background())
		defer cancelFn()

		// Run the fetch
		require.NoError(t, f.FetchChainData(ctx))

		for blockIndex := 1; blockIndex < blockNum; blockIndex++ {
			assert.Equal(t, blocks[blockIndex], savedBlocks[blockIndex])
		}

		// Make sure proper events were emitted
		require.Len(t, capturedEvents, len(blocks))

		for index, event := range capturedEvents {
			if index == 0 {
				// Dummy genesis block
				continue
			}

			// Make sure the block is valid
			assert.Equal(t, blocks[index], event.Block)

			// Make sure the transaction results are valid
			require.Len(t, event.Results, 0)
		}
	})

	t.Run("no txs in block, batch", func(t *testing.T) {
		t.Parallel()

		var cancelFn context.CancelFunc

		var (
			blockNum = 5
			blocks   = generateBlocks(t, blockNum+1, []*std.Tx{})

			savedBlocks    = make([]*types.Block, 0, blockNum)
			capturedEvents = make([]*indexerTypes.NewBlock, 0)

			mockEvents = &mockEvents{
				signalEventFn: func(e events.Event) {
					blockEvent, ok := e.(*indexerTypes.NewBlock)
					require.True(t, ok)

					capturedEvents = append(capturedEvents, blockEvent)
				},
			}

			mockStorage = &mock.Storage{
				GetLatestSavedHeightFn: func() (uint64, error) {
					return 0, storageErrors.ErrNotFound
				},
				GetWriteBatchFn: func() storage.Batch {
					return &mock.WriteBatch{
						SetBlockFn: func(block *types.Block) error {
							savedBlocks = append(savedBlocks, block)

							// Check if all blocks are saved
							if block.Height == int64(blockNum) {
								// At this point, we can cancel the process
								cancelFn()
							}

							return nil
						},
						SetTxFn: func(_ *types.TxResult) error {
							t.Fatalf("should not save txs")

							return nil
						},
					}
				},
			}

			batch = make([]any, 0)

			mockClient = &mockClient{
				createBatchFn: func() clientTypes.Batch {
					return &mockBatch{
						executeFn: func(_ context.Context) ([]any, error) {
							results := make([]any, len(batch))
							copy(results, batch)

							batch = batch[:0]

							return results, nil
						},
						countFn: func() int {
							return len(batch)
						},
						addBlockRequestFn: func(num uint64) error {
							// Sanity check
							if num > uint64(blockNum) {
								t.Fatalf("invalid block requested, %d", num)
							}

							batch = append(
								batch,
								&core_types.ResultBlock{
									Block: blocks[num],
								},
							)

							return nil
						},
						addBlockResultsRequestFn: func(num uint64) error {
							t.Fatalf("block %d should not have txs", num)

							return nil
						},
					}
				},
				getBlockResultsFn: func(num uint64) (*core_types.ResultBlockResults, error) {
					if num == 0 {
						return &core_types.ResultBlockResults{
							Height: int64(num),
							Results: &state.ABCIResponses{
								DeliverTxs: make([]abci.ResponseDeliverTx, 0),
							},
						}, nil
					}

					t.Fatalf("should not request results")

					return nil, nil
				},
				getLatestBlockNumberFn: func() (uint64, error) {
					return uint64(blockNum), nil
				},
				getGenesisFn: func() (*core_types.ResultGenesis, error) {
					return &core_types.ResultGenesis{
						Genesis: &types.GenesisDoc{
							AppState: gnoland.GnoGenesisState{
								Balances: []gnoland.Balance{},
								Txs:      []gnoland.TxWithMetadata{},
							},
						},
					}, nil
				},
			}
		)

		// Create the fetcher
		f := New(mockStorage, mockClient, mockEvents)

		// Create the context
		ctx, cancelFn := context.WithCancel(context.Background())
		defer cancelFn()

		// Run the fetch
		require.NoError(t, f.FetchChainData(ctx))

		for blockIndex := 1; blockIndex < blockNum; blockIndex++ {
			assert.Equal(t, blocks[blockIndex], savedBlocks[blockIndex])
		}

		// Make sure proper events were emitted
		require.Len(t, capturedEvents, len(blocks))

		for index, event := range capturedEvents {
			if index == 0 {
				// Dummy genesis block
				continue
			}

			// Make sure the block is valid
			assert.Equal(t, blocks[index], event.Block)

			// Make sure the transaction results are valid
			require.Len(t, event.Results, 0)
		}
	})
}

func TestFetcher_InvalidBlocks(t *testing.T) {
	t.Parallel()

	var cancelFn context.CancelFunc

	var (
		blockNum = 10
		txCount  = 1
		txs      = generateTransactions(t, txCount)
		blocks   = generateBlocks(t, blockNum+1, txs)

		savedBlocks    = make([]*types.Block, 0, blockNum)
		capturedEvents = make([]*indexerTypes.NewBlock, 0)

		mockEvents = &mockEvents{
			signalEventFn: func(e events.Event) {
				blockEvent, ok := e.(*indexerTypes.NewBlock)
				require.True(t, ok)

				capturedEvents = append(capturedEvents, blockEvent)
			},
		}

		mockStorage = &mock.Storage{
			GetLatestSavedHeightFn: func() (uint64, error) {
				return 0, storageErrors.ErrNotFound
			},
			GetWriteBatchFn: func() storage.Batch {
				return &mock.WriteBatch{
					SetBlockFn: func(block *types.Block) error {
						savedBlocks = append(savedBlocks, block)

						// Check if all blocks are saved
						if block.Height == int64(blockNum) {
							// At this point, we can cancel the process
							cancelFn()
						}

						return fmt.Errorf("unable to save block %d", block.Height)
					},
					SetTxFn: func(_ *types.TxResult) error {
						t.Fatalf("should not save txs")

						return nil
					},
				}
			},
		}

		mockClient = &mockClient{
			createBatchFn: func() clientTypes.Batch {
				return &mockBatch{
					executeFn: func(_ context.Context) ([]any, error) {
						// Force an error
						return nil, errors.New("something is flaky")
					},
					countFn: func() int {
						return 1 // to trigger execution
					},
				}
			},
			getLatestBlockNumberFn: func() (uint64, error) {
				return uint64(blockNum), nil
			},
			getBlockFn: func(num uint64) (*core_types.ResultBlock, error) {
				// Sanity check
				if num > uint64(blockNum) {
					t.Fatalf("invalid block requested, %d", num)
				}

				return &core_types.ResultBlock{
					Block: blocks[num],
				}, nil
			},
			getBlockResultsFn: func(num uint64) (*core_types.ResultBlockResults, error) {
				if num == 0 {
					return &core_types.ResultBlockResults{
						Height: int64(num),
						Results: &state.ABCIResponses{
							DeliverTxs: make([]abci.ResponseDeliverTx, 0),
						},
					}, nil
				}

				require.LessOrEqual(t, num, uint64(blockNum))

				return nil, fmt.Errorf("unable to fetch result for block %d", num)
			},
			getGenesisFn: func() (*core_types.ResultGenesis, error) {
				return &core_types.ResultGenesis{
					Genesis: &types.GenesisDoc{
						AppState: gnoland.GnoGenesisState{
							Balances: []gnoland.Balance{},
							Txs:      []gnoland.TxWithMetadata{},
						},
					},
				}, nil
			},
		}
	)

	// Create the fetcher
	f := New(mockStorage, mockClient, mockEvents)

	// Create the context
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	// Run the fetch
	require.NoError(t, f.FetchChainData(ctx))

	// Make sure correct blocks were attempted to be saved
	for blockIndex := 1; blockIndex < blockNum; blockIndex++ {
		assert.Equal(t, blocks[blockIndex], savedBlocks[blockIndex])
	}

	// Make sure no events were emitted
	assert.Len(t, capturedEvents, 0)
}

func TestFetcher_Genesis(t *testing.T) {
	t.Parallel()

	var (
		txCount     = 21
		txs         = generateGenesisTransactions(t, txCount)
		savedBlocks = map[int64]*types.Block{}
		savedTxs    = map[string]*types.TxResult{}

		capturedEvents = make([]*indexerTypes.NewBlock, 0)

		mockEvents = &mockEvents{
			signalEventFn: func(e events.Event) {
				blockEvent, ok := e.(*indexerTypes.NewBlock)
				require.True(t, ok)

				capturedEvents = append(capturedEvents, blockEvent)
			},
		}

		mockStorage = &mock.Storage{
			GetLatestSavedHeightFn: func() (uint64, error) {
				return 0, storageErrors.ErrNotFound
			},
			GetWriteBatchFn: func() storage.Batch {
				return &mock.WriteBatch{
					SetBlockFn: func(block *types.Block) error {
						_, ok := savedBlocks[block.Height]
						require.False(t, ok)
						savedBlocks[block.Height] = block

						return nil
					},
					SetTxFn: func(tx *types.TxResult) error {
						savedTxs[fmt.Sprintf("%d-%d", tx.Height, tx.Index)] = tx

						return nil
					},
				}
			},
		}

		mockClient = &mockClient{
			getLatestBlockNumberFn: func() (uint64, error) {
				return 0, nil
			},
			getGenesisFn: func() (*core_types.ResultGenesis, error) {
				localTxs := make([]gnoland.TxWithMetadata, len(txs))
				for i, tx := range txs {
					localTxs[i] = *tx
				}

				return &core_types.ResultGenesis{Genesis: &types.GenesisDoc{AppState: gnoland.GnoGenesisState{
					Txs: localTxs,
				}}}, nil
			},
			getBlockResultsFn: func(uint64) (*core_types.ResultBlockResults, error) {
				return &core_types.ResultBlockResults{
					Results: &state.ABCIResponses{
						DeliverTxs: make([]abci.ResponseDeliverTx, len(txs)),
					},
				}, nil
			},
		}
	)

	// Create the context
	ctx := context.Background()

	f := New(mockStorage, mockClient, mockEvents)

	require.NoError(t, f.fetchGenesisData(ctx))

	require.Len(t, capturedEvents, 1)

	_, ok := savedBlocks[0]
	require.True(t, ok)

	for i := uint32(0); i < uint32(len(txs)); i++ {
		tx, ok := savedTxs[fmt.Sprintf("0-%d", i)]
		require.True(t, ok)

		expected := &types.TxResult{
			Height:   0,
			Index:    i,
			Tx:       amino.MustMarshal(txs[i].Tx),
			Response: abci.ResponseDeliverTx{},
		}
		require.Equal(t, expected, tx)
	}
}

func TestFetcher_GenesisAlreadyFetched(t *testing.T) {
	t.Parallel()

	var (
		mockEvents = &mockEvents{}

		mockStorage = &mock.Storage{
			GetLatestSavedHeightFn: func() (uint64, error) {
				return 0, nil
			},
		}

		mockClient = &mockClient{}
	)

	// Create the context
	ctx := context.Background()

	f := New(mockStorage, mockClient, mockEvents)

	require.NoError(t, f.fetchGenesisData(ctx))
}

func TestFetcher_GenesisFetchError(t *testing.T) {
	t.Parallel()

	var (
		remoteErr = errors.New("remote error")

		mockEvents = &mockEvents{
			signalEventFn: func(_ events.Event) {
				require.Fail(t, "should not emit events")
			},
		}

		mockStorage = &mock.Storage{
			GetLatestSavedHeightFn: func() (uint64, error) {
				return 0, storageErrors.ErrNotFound
			},
			GetWriteBatchFn: func() storage.Batch {
				require.Fail(t, "should not attempt to write to storage")

				return nil
			},
		}

		mockClient = &mockClient{
			getLatestBlockNumberFn: func() (uint64, error) {
				return 0, nil
			},
			getGenesisFn: func() (*core_types.ResultGenesis, error) {
				return nil, remoteErr
			},
			getBlockResultsFn: func(uint64) (*core_types.ResultBlockResults, error) {
				require.Fail(t, "should not attempt to fetch block results")

				return nil, nil
			},
		}
	)

	// Create the context
	ctx := context.Background()

	f := New(mockStorage, mockClient, mockEvents)

	require.ErrorIs(t, f.fetchGenesisData(ctx), remoteErr)
}

func TestFetcher_GenesisInvalidState(t *testing.T) {
	t.Parallel()

	var (
		mockEvents = &mockEvents{
			signalEventFn: func(_ events.Event) {
				require.Fail(t, "should not emit events")
			},
		}

		mockStorage = &mock.Storage{
			GetLatestSavedHeightFn: func() (uint64, error) {
				return 0, storageErrors.ErrNotFound
			},
			GetWriteBatchFn: func() storage.Batch {
				require.Fail(t, "should not attempt to write to storage")

				return nil
			},
		}

		mockClient = &mockClient{
			getLatestBlockNumberFn: func() (uint64, error) {
				return 0, nil
			},
			getGenesisFn: func() (*core_types.ResultGenesis, error) {
				return &core_types.ResultGenesis{Genesis: &types.GenesisDoc{AppState: 0xdeadbeef}}, nil
			},
			getBlockResultsFn: func(uint64) (*core_types.ResultBlockResults, error) {
				require.Fail(t, "should not attempt to fetch block results")

				return nil, nil
			},
		}
	)

	// Create the context
	ctx := context.Background()

	f := New(mockStorage, mockClient, mockEvents)

	require.ErrorContains(t, f.fetchGenesisData(ctx), "unknown genesis state kind 'int'")
}

func TestFetcher_GenesisFetchResultsError(t *testing.T) {
	t.Parallel()

	var (
		remoteErr = errors.New("remote error")

		mockEvents = &mockEvents{
			signalEventFn: func(_ events.Event) {
				require.Fail(t, "should not emit events")
			},
		}

		mockStorage = &mock.Storage{
			GetLatestSavedHeightFn: func() (uint64, error) {
				return 0, storageErrors.ErrNotFound
			},
			GetWriteBatchFn: func() storage.Batch {
				require.Fail(t, "should not attempt to write to storage")

				return nil
			},
		}

		mockClient = &mockClient{
			getLatestBlockNumberFn: func() (uint64, error) {
				return 0, nil
			},
			getGenesisFn: func() (*core_types.ResultGenesis, error) {
				return &core_types.ResultGenesis{Genesis: &types.GenesisDoc{
					AppState: gnoland.GnoGenesisState{Txs: []gnoland.TxWithMetadata{{}}},
				}}, nil
			},
			getBlockResultsFn: func(uint64) (*core_types.ResultBlockResults, error) {
				return nil, remoteErr
			},
		}
	)

	// Create the context
	ctx := context.Background()

	f := New(mockStorage, mockClient, mockEvents)

	require.ErrorIs(t, f.fetchGenesisData(ctx), remoteErr)
}

func TestFetcher_GenesisNilGenesisDoc(t *testing.T) {
	t.Parallel()

	var (
		mockEvents = &mockEvents{
			signalEventFn: func(_ events.Event) {
				require.Fail(t, "should not emit events")
			},
		}

		mockStorage = &mock.Storage{
			GetLatestSavedHeightFn: func() (uint64, error) {
				return 0, storageErrors.ErrNotFound
			},
			GetWriteBatchFn: func() storage.Batch {
				require.Fail(t, "should not attempt to write")

				return nil
			},
		}

		mockClient = &mockClient{
			getLatestBlockNumberFn: func() (uint64, error) {
				return 0, nil
			},
			getGenesisFn: func() (*core_types.ResultGenesis, error) {
				return &core_types.ResultGenesis{Genesis: nil}, nil
			},
			getBlockResultsFn: func(uint64) (*core_types.ResultBlockResults, error) {
				return &core_types.ResultBlockResults{Results: &state.ABCIResponses{}}, nil
			},
		}
	)

	// Create the context
	ctx := context.Background()

	f := New(mockStorage, mockClient, mockEvents)

	require.Error(t, f.fetchGenesisData(ctx))
}

func TestFetcher_GenesisNilResults(t *testing.T) {
	t.Parallel()

	var (
		mockEvents = &mockEvents{
			signalEventFn: func(_ events.Event) {
				require.Fail(t, "should not emit events")
			},
		}

		mockStorage = &mock.Storage{
			GetLatestSavedHeightFn: func() (uint64, error) {
				return 0, storageErrors.ErrNotFound
			},
			GetWriteBatchFn: func() storage.Batch {
				require.Fail(t, "should not attempt to write")

				return nil
			},
		}

		mockClient = &mockClient{
			getLatestBlockNumberFn: func() (uint64, error) {
				return 0, nil
			},
			getGenesisFn: func() (*core_types.ResultGenesis, error) {
				return &core_types.ResultGenesis{Genesis: &types.GenesisDoc{
					AppState: gnoland.GnoGenesisState{Txs: []gnoland.TxWithMetadata{{}}},
				}}, nil
			},
			getBlockResultsFn: func(uint64) (*core_types.ResultBlockResults, error) {
				return &core_types.ResultBlockResults{Results: nil}, nil
			},
		}
	)

	// Create the context
	ctx := context.Background()

	f := New(mockStorage, mockClient, mockEvents)

	require.Error(t, f.fetchGenesisData(ctx))
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

// generateGenesisTransactions generates dummy genesis transactions
func generateGenesisTransactions(t *testing.T, count int) []*gnoland.TxWithMetadata {
	t.Helper()

	txs := make([]*gnoland.TxWithMetadata, count)

	for i := 0; i < count; i++ {
		txs[i] = &gnoland.TxWithMetadata{
			Tx: std.Tx{
				Memo: fmt.Sprintf("memo %d", i),
			},
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
		serializedTx, err := amino.Marshal(tx)
		require.NoError(t, err)

		serializedTxs = append(serializedTxs, serializedTx)
	}

	return serializedTxs
}
