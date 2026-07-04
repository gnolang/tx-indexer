package fetch

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
	"github.com/gnolang/tx-indexer/genesis"
	"github.com/gnolang/tx-indexer/internal/mock"
	"github.com/gnolang/tx-indexer/storage"
	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
	indexerTypes "github.com/gnolang/tx-indexer/types"
)

// bootstrapGenesis runs the genesis bootstrap the way cmd/start.go does, so a
// test's fetched blocks follow block 0 exactly as in production. Bounded on
// purpose: the real bootstrap retries until it succeeds, which inside a test
// would be a hang rather than a failure.
func bootstrapGenesis(t *testing.T, store genesis.Storage, client genesis.Client) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, genesis.Bootstrap(
		ctx,
		store,
		client,
		genesis.WithBackoff(time.Millisecond),
	))
}

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
		// Bootstrap the genesis first, the way cmd/start.go does, so the
		// genesis block precedes the fetched blocks exactly as in production.
		bootstrapGenesis(t, mockStorage, mockClient)

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

		// Make sure proper events were emitted. One per fetched block: the
		// genesis block is indexed by the genesis package, which signals
		// nothing because no subscriber exists before the HTTP server starts.
		require.Len(t, capturedEvents, blockNum)

		for index, event := range capturedEvents {
			// capturedEvents[0] is block 1
			blockIndex := index + 1

			if event.GetType() != indexerTypes.NewBlockEvent {
				continue
			}

			eventData, ok := event.(*indexerTypes.NewBlock)
			require.True(t, ok)

			// Make sure the block is valid
			assert.Equal(t, blocks[blockIndex], eventData.Block)

			// Make sure the transaction results are valid
			require.Len(t, eventData.Results, txCount)

			for txIndex, tx := range eventData.Results {
				assert.EqualValues(t, blocks[blockIndex].Height, tx.Height)
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

							batch = append(
								batch,
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
		// Bootstrap the genesis first, the way cmd/start.go does, so the
		// genesis block precedes the fetched blocks exactly as in production.
		bootstrapGenesis(t, mockStorage, mockClient)

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

		// Make sure proper events were emitted. One per fetched block: the
		// genesis block is indexed by the genesis package, which signals
		// nothing because no subscriber exists before the HTTP server starts.
		require.Len(t, capturedEvents, blockNum)

		for index, event := range capturedEvents {
			// capturedEvents[0] is block 1
			blockIndex := index + 1

			// Make sure the block is valid
			eventData := event.(*indexerTypes.NewBlock)
			assert.Equal(t, blocks[blockIndex], eventData.Block)

			// Make sure the transaction results are valid
			require.Len(t, eventData.Results, txCount)

			for txIndex, tx := range eventData.Results {
				assert.EqualValues(t, blocks[blockIndex].Height, tx.Height)
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
		// Bootstrap the genesis first, the way cmd/start.go does, so the
		// genesis block precedes the fetched blocks exactly as in production.
		bootstrapGenesis(t, mockStorage, mockClient)

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

		// Make sure proper events were emitted. One per fetched block: the
		// genesis block is indexed by the genesis package, which signals
		// nothing because no subscriber exists before the HTTP server starts.
		// Blocks each have as many transactions as txCount.
		require.Len(t, capturedEvents, blockNum)

		for index, event := range capturedEvents {
			// capturedEvents[0] is block 1
			blockIndex := index + 1

			if event.GetType() != indexerTypes.NewBlockEvent {
				continue
			}

			eventData, ok := event.(*indexerTypes.NewBlock)
			require.True(t, ok)

			// Make sure the block is valid
			assert.Equal(t, blocks[blockIndex], eventData.Block)

			// Make sure the transaction results are valid
			require.Len(t, eventData.Results, txCount)

			for txIndex, tx := range eventData.Results {
				assert.EqualValues(t, blocks[blockIndex].Height, tx.Height)
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
		// Bootstrap the genesis first, the way cmd/start.go does, so the
		// genesis block precedes the fetched blocks exactly as in production.
		bootstrapGenesis(t, mockStorage, mockClient)

		require.NoError(t, f.FetchChainData(ctx))

		for blockIndex := 1; blockIndex < blockNum; blockIndex++ {
			assert.Equal(t, blocks[blockIndex], savedBlocks[blockIndex])
		}

		// Make sure proper events were emitted. One per fetched block: the
		// genesis block is indexed by the genesis package, which signals
		// nothing because no subscriber exists before the HTTP server starts.
		require.Len(t, capturedEvents, blockNum)

		for index, event := range capturedEvents {
			// capturedEvents[0] is block 1
			// Make sure the block is valid
			assert.Equal(t, blocks[index+1], event.Block)

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
		// Bootstrap the genesis first, the way cmd/start.go does, so the
		// genesis block precedes the fetched blocks exactly as in production.
		bootstrapGenesis(t, mockStorage, mockClient)

		require.NoError(t, f.FetchChainData(ctx))

		for blockIndex := 1; blockIndex < blockNum; blockIndex++ {
			assert.Equal(t, blocks[blockIndex], savedBlocks[blockIndex])
		}

		// Make sure proper events were emitted. One per fetched block: the
		// genesis block is indexed by the genesis package, which signals
		// nothing because no subscriber exists before the HTTP server starts.
		require.Len(t, capturedEvents, blockNum)

		for index, event := range capturedEvents {
			// capturedEvents[0] is block 1
			// Make sure the block is valid
			assert.Equal(t, blocks[index+1], event.Block)

			// Make sure the transaction results are valid
			require.Len(t, event.Results, 0)
		}
	})
}

// TestFetcher_InvalidBlocks covers blocks the storage layer refuses, which are
// skipped so that a chain carrying data an older Amino cannot decode does not
// stop the fetcher. The chunk itself is fetched cleanly here: a failed fetch is
// refetched rather than saved, and is covered by
// TestFetcher_ChunkFetchError_NoSilentGap
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
				require.LessOrEqual(t, num, uint64(blockNum))

				// The genesis block carries no transactions
				deliverTxs := txCount
				if num == 0 {
					deliverTxs = 0
				}

				return &core_types.ResultBlockResults{
					Height: int64(num),
					Results: &state.ABCIResponses{
						DeliverTxs: make([]abci.ResponseDeliverTx, deliverTxs),
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
	f := New(mockStorage, mockClient, mockEvents)

	// Create the context
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	// Run the fetch. No genesis bootstrap here: this storage refuses every
	// block, which the fetcher tolerates by design but the genesis bootstrap
	// rightly does not — so the fetcher starts from height 1.
	require.NoError(t, f.FetchChainData(ctx))

	// Make sure correct blocks were attempted to be saved
	for blockIndex := 1; blockIndex < blockNum; blockIndex++ {
		assert.Equal(t, blocks[blockIndex], savedBlocks[blockIndex-1])
	}

	// Make sure no events were emitted
	assert.Len(t, capturedEvents, 0)
}

func TestFetcher_ShutdownWithInFlightWorkers(t *testing.T) {
	t.Parallel()

	const (
		blockNum  = 20
		chunkSize = 5
		// Whether a late delivery trips on the shutdown is a coin flip per
		// worker, so the scenario is repeated to make the outcome reliable
		runs = 8
	)

	var (
		txs    = generateTransactions(t, 1)
		blocks = generateBlocks(t, blockNum+1, txs)
	)

	for run := 0; run < runs; run++ {
		var (
			cancelFn context.CancelFunc

			// Released once FetchChainData has returned, so every gated
			// worker delivers its response strictly after the shutdown
			gate       = make(chan struct{})
			cancelOnce sync.Once
		)

		mockStorage := &mock.Storage{
			GetLatestSavedHeightFn: func() (uint64, error) {
				return 0, storageErrors.ErrNotFound
			},
			GetWriteBatchFn: func() storage.Batch {
				return &mock.WriteBatch{}
			},
		}

		mockClient := &mockClient{
			createBatchFn: func() clientTypes.Batch {
				return &mockBatch{
					executeFn: func(_ context.Context) ([]any, error) {
						// Force the sequential fetch path
						return nil, errors.New("batch unavailable")
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
				return &core_types.ResultBlock{
					Block: blocks[num],
				}, nil
			},
			getBlockResultsFn: func(num uint64) (*core_types.ResultBlockResults, error) {
				// The genesis fetch runs before the worker loop starts
				if num == 0 {
					return &core_types.ResultBlockResults{
						Height:  0,
						Results: &state.ABCIResponses{},
					}, nil
				}

				// The first worker to get here shuts the fetcher down; every
				// worker then holds its response until the shutdown completes
				cancelOnce.Do(cancelFn)
				<-gate

				return nil, errors.New("could not find results for height")
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

		// Create the fetcher
		f := New(
			mockStorage,
			mockClient,
			&mockEvents{},
			WithLogger(zap.NewNop()),
		)

		// Small chunks so the range fans out into several workers
		f.maxChunkSize = chunkSize

		// The timeout is a backstop: the first worker to fetch block results
		// shuts the fetcher down
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		cancelFn = cancel

		// Bootstrap the genesis first, the way cmd/start.go does, so the
		// genesis block precedes the fetched blocks exactly as in production.
		bootstrapGenesis(t, mockStorage, mockClient)

		require.NoError(t, f.FetchChainData(ctx))

		close(gate)
		cancel()
	}
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
		serializedTx, err := amino.Marshal(tx)
		require.NoError(t, err)

		serializedTxs = append(serializedTxs, serializedTx)
	}

	return serializedTxs
}
