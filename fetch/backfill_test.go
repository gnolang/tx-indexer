package fetch

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	clientTypes "github.com/gnolang/tx-indexer/client/types"
	"github.com/gnolang/tx-indexer/internal/mock"
	"github.com/gnolang/tx-indexer/storage"
	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
)

// fastRetry is a retry policy with a negligible delay, for tests.
var fastRetry = retryConfig{
	maxRetries: 5,
	retryDelay: time.Millisecond,
}

// erroringBatch returns a batch whose Execute always fails, forcing the
// sequential fetch fallback (which is the path that exercises retries).
func erroringBatch() clientTypes.Batch {
	return &mockBatch{
		executeFn: func(_ context.Context) ([]any, error) {
			return nil, errors.New("batch unavailable")
		},
		countFn: func() int { return 1 },
	}
}

// blocksAtHeights builds empty blocks at the given heights.
func blocksAtHeights(heights ...int64) []*types.Block {
	blocks := make([]*types.Block, len(heights))
	for i, h := range heights {
		blocks[i] = &types.Block{Header: types.Header{Height: h}}
	}

	return blocks
}

func TestGapTracker(t *testing.T) {
	t.Parallel()

	g := newGapTracker()
	require.Equal(t, 0, g.len())

	g.add(3, 1, 2, 1) // duplicate 1 collapses
	require.Equal(t, 3, g.len())
	assert.Equal(t, []uint64{1, 2, 3}, g.snapshot())

	g.remove(2)
	assert.Equal(t, []uint64{1, 3}, g.snapshot())
}

func TestAuditGaps(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name         string
		stored       []int64
		latestErr    error
		expectedGaps []uint64
		latest       uint64
	}{
		{"nothing indexed yet", nil, storageErrors.ErrNotFound, []uint64{}, 0},
		{"no gaps", []int64{0, 1, 2, 3}, nil, []uint64{}, 3},
		{"leading gap", []int64{2, 3}, nil, []uint64{0, 1}, 3},
		{"middle gaps", []int64{0, 1, 3, 5}, nil, []uint64{2, 4}, 5},
		{"trailing gap", []int64{0, 1, 2}, nil, []uint64{3, 4, 5}, 5},
		{"only genesis stored", []int64{0}, nil, []uint64{1, 2, 3}, 3},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			storageMock := &mock.Storage{
				GetLatestSavedHeightFn: func() (uint64, error) {
					return testCase.latest, testCase.latestErr
				},
				BlockIteratorFn: func(_, _ uint64) (storage.Iterator[*types.Block], error) {
					return mock.NewSliceBlockIterator(blocksAtHeights(testCase.stored...)), nil
				},
			}

			f := New(storageMock, &mockClient{}, &mockEvents{})

			require.NoError(t, f.auditGaps(context.Background()))
			assert.Equal(t, testCase.expectedGaps, f.gaps.snapshot())
		})
	}
}

// heightScannerStorage is a storage that also implements blockHeightScanner,
// used to verify auditGaps prefers the cheap key-only scan.
type heightScannerStorage struct {
	*mock.Storage

	heightIter func(uint64, uint64) (storage.Iterator[uint64], error)
}

func (h *heightScannerStorage) BlockHeightIterator(from, to uint64) (storage.Iterator[uint64], error) {
	return h.heightIter(from, to)
}

// uint64SliceIter is an in-memory Iterator[uint64] for tests.
type uint64SliceIter struct {
	vals  []uint64
	index int
}

func newUint64SliceIter(vals []uint64) *uint64SliceIter {
	return &uint64SliceIter{vals: vals, index: -1}
}

func (it *uint64SliceIter) Next() bool {
	it.index++

	return it.index < len(it.vals)
}

func (it *uint64SliceIter) Value() (uint64, error) { return it.vals[it.index], nil }
func (it *uint64SliceIter) Error() error           { return nil }
func (it *uint64SliceIter) Close() error           { return nil }

func TestAuditGaps_PrefersCheapHeightScan(t *testing.T) {
	t.Parallel()

	storageMock := &heightScannerStorage{
		Storage: &mock.Storage{
			GetLatestSavedHeightFn: func() (uint64, error) { return 3, nil },
			BlockIteratorFn: func(_, _ uint64) (storage.Iterator[*types.Block], error) {
				t.Fatal("BlockIterator must not be used when the cheap scan is available")

				return nil, nil
			},
		},
		heightIter: func(_, _ uint64) (storage.Iterator[uint64], error) {
			return newUint64SliceIter([]uint64{0, 2}), nil // missing 1 and 3
		},
	}

	f := New(storageMock, &mockClient{}, &mockEvents{})

	require.NoError(t, f.auditGaps(context.Background()))
	assert.Equal(t, []uint64{1, 3}, f.gaps.snapshot())
}

func TestAuditTxGaps(t *testing.T) {
	t.Parallel()

	type blockSpec struct {
		height int64
		numTxs int64
	}

	testTable := []struct {
		name         string
		blocks       []blockSpec
		txHeights    []int64 // one entry per stored tx, ascending
		latestErr    error
		expectedGaps []uint64
		latest       uint64
	}{
		{"nothing indexed yet", nil, nil, storageErrors.ErrNotFound, []uint64{}, 0},
		{
			"all complete",
			[]blockSpec{{1, 2}, {2, 1}},
			[]int64{1, 1, 2},
			nil,
			[]uint64{},
			2,
		},
		{
			"partial and fully missing",
			[]blockSpec{{1, 2}, {2, 2}, {3, 2}},
			[]int64{1, 1, 2},
			nil,
			[]uint64{2, 3},
			3,
		},
		{
			"empty blocks are skipped",
			[]blockSpec{{1, 0}, {2, 0}},
			nil,
			nil,
			[]uint64{},
			2,
		},
		{
			"trailing block incomplete",
			[]blockSpec{{1, 1}, {2, 2}},
			[]int64{1, 2},
			nil,
			[]uint64{2},
			2,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			blocks := make([]*types.Block, len(testCase.blocks))
			for i, spec := range testCase.blocks {
				blocks[i] = &types.Block{
					Header: types.Header{Height: spec.height, NumTxs: spec.numTxs},
				}
			}

			txs := make([]*types.TxResult, len(testCase.txHeights))
			for i, h := range testCase.txHeights {
				txs[i] = &types.TxResult{Height: h}
			}

			storageMock := &mock.Storage{
				GetLatestSavedHeightFn: func() (uint64, error) {
					return testCase.latest, testCase.latestErr
				},
				BlockIteratorFn: func(_, _ uint64) (storage.Iterator[*types.Block], error) {
					return mock.NewSliceBlockIterator(blocks), nil
				},
				TxIteratorFn: func(_, _ uint64, _, _ uint32) (storage.Iterator[*types.TxResult], error) {
					return mock.NewSliceTxIterator(txs), nil
				},
			}

			f := New(storageMock, &mockClient{}, &mockEvents{})

			require.NoError(t, f.auditTxGaps(context.Background()))
			assert.Equal(t, testCase.expectedGaps, f.gaps.snapshot())
		})
	}
}

// blocksInRange / txsInRange let the mock iterators honor the window bounds the
// windowed tx audit passes in.
func blocksInRange(blocks []*types.Block, from, to uint64) []*types.Block {
	out := make([]*types.Block, 0, len(blocks))
	for _, b := range blocks {
		if uint64(b.Height) >= from && uint64(b.Height) <= to {
			out = append(out, b)
		}
	}

	return out
}

func txsInRange(txs []*types.TxResult, from, to uint64) []*types.TxResult {
	out := make([]*types.TxResult, 0, len(txs))
	for _, tx := range txs {
		if uint64(tx.Height) >= from && uint64(tx.Height) <= to {
			out = append(out, tx)
		}
	}

	return out
}

// txAuditBlocks builds single-tx blocks at the given heights.
func txAuditBlocks(heights ...int64) []*types.Block {
	blocks := make([]*types.Block, len(heights))
	for i, h := range heights {
		blocks[i] = &types.Block{Header: types.Header{Height: h, NumTxs: 1}}
	}

	return blocks
}

func TestAuditTxGaps_HonorsFromHeight(t *testing.T) {
	t.Parallel()

	// Heights 1 and 3 declare a tx but none are stored (both incomplete).
	blocks := txAuditBlocks(1, 3)

	storageMock := &mock.Storage{
		GetLatestSavedHeightFn: func() (uint64, error) { return 3, nil },
		BlockIteratorFn: func(from, to uint64) (storage.Iterator[*types.Block], error) {
			return mock.NewSliceBlockIterator(blocksInRange(blocks, from, to)), nil
		},
		TxIteratorFn: func(_, _ uint64, _, _ uint32) (storage.Iterator[*types.TxResult], error) {
			return mock.NewSliceTxIterator(nil), nil
		},
	}

	// Floor at height 2: height 1 must be skipped, only 3 is queued.
	f := New(storageMock, &mockClient{}, &mockEvents{}, WithAuditFromHeight(2))

	require.NoError(t, f.auditTxGaps(context.Background()))
	assert.Equal(t, []uint64{3}, f.gaps.snapshot())
}

func TestAuditTxGaps_ResumesFromWatermark(t *testing.T) {
	t.Parallel()

	// All incomplete, but the watermark says heights <= 2 are already verified.
	blocks := txAuditBlocks(1, 2, 3)
	watermark := uint64(2)

	storageMock := &mock.Storage{
		GetLatestSavedHeightFn: func() (uint64, error) { return 3, nil },
		BlockIteratorFn: func(from, to uint64) (storage.Iterator[*types.Block], error) {
			return mock.NewSliceBlockIterator(blocksInRange(blocks, from, to)), nil
		},
		TxIteratorFn: func(_, _ uint64, _, _ uint32) (storage.Iterator[*types.TxResult], error) {
			return mock.NewSliceTxIterator(nil), nil
		},
		GetTxAuditHeightFn: func() (uint64, error) { return watermark, nil },
	}

	f := New(storageMock, &mockClient{}, &mockEvents{})

	require.NoError(t, f.auditTxGaps(context.Background()))
	assert.Equal(t, []uint64{3}, f.gaps.snapshot(), "heights at or below the watermark must be skipped")
}

func TestAuditTxGaps_PersistsWatermarkPerWindow(t *testing.T) {
	t.Parallel()

	t.Run("advances to latest when everything is complete", func(t *testing.T) {
		t.Parallel()

		blocks := txAuditBlocks(1, 2, 3, 4)
		txs := []*types.TxResult{{Height: 1}, {Height: 2}, {Height: 3}, {Height: 4}}

		var persisted []uint64

		storageMock := &mock.Storage{
			GetLatestSavedHeightFn: func() (uint64, error) { return 4, nil },
			BlockIteratorFn: func(from, to uint64) (storage.Iterator[*types.Block], error) {
				return mock.NewSliceBlockIterator(blocksInRange(blocks, from, to)), nil
			},
			TxIteratorFn: func(from, to uint64, _, _ uint32) (storage.Iterator[*types.TxResult], error) {
				return mock.NewSliceTxIterator(txsInRange(txs, from, to)), nil
			},
			SetTxAuditHeightFn: func(h uint64) error {
				persisted = append(persisted, h)

				return nil
			},
		}

		// from height 1, window of 2 -> [1,2] then [3,4]
		f := New(storageMock, &mockClient{}, &mockEvents{}, WithAuditFromHeight(1), WithTxAuditThrottle(2, 0))

		require.NoError(t, f.auditTxGaps(context.Background()))
		assert.Empty(t, f.gaps.snapshot())
		assert.Equal(t, []uint64{2, 4}, persisted, "watermark advances per completed window")
	})

	t.Run("freezes just below the first incomplete height", func(t *testing.T) {
		t.Parallel()

		// Height 3 is incomplete; 1, 2, 4 are complete.
		blocks := txAuditBlocks(1, 2, 3, 4)
		txs := []*types.TxResult{{Height: 1}, {Height: 2}, {Height: 4}}

		var persisted []uint64

		storageMock := &mock.Storage{
			GetLatestSavedHeightFn: func() (uint64, error) { return 4, nil },
			BlockIteratorFn: func(from, to uint64) (storage.Iterator[*types.Block], error) {
				return mock.NewSliceBlockIterator(blocksInRange(blocks, from, to)), nil
			},
			TxIteratorFn: func(from, to uint64, _, _ uint32) (storage.Iterator[*types.TxResult], error) {
				return mock.NewSliceTxIterator(txsInRange(txs, from, to)), nil
			},
			SetTxAuditHeightFn: func(h uint64) error {
				persisted = append(persisted, h)

				return nil
			},
		}

		// from height 1, window of 1 -> [1],[2],[3],[4]
		f := New(storageMock, &mockClient{}, &mockEvents{}, WithAuditFromHeight(1), WithTxAuditThrottle(1, 0))

		require.NoError(t, f.auditTxGaps(context.Background()))
		assert.Equal(t, []uint64{3}, f.gaps.snapshot())
		// 1 and 2 complete -> persist 1, 2; then height 3 incomplete freezes at 2;
		// nothing persisted for 3 or 4.
		assert.Equal(t, []uint64{1, 2, 2}, persisted)
	})
}

func TestWriteSlot_QueuesFailedHeights(t *testing.T) {
	t.Parallel()

	txs := generateTransactions(t, 2)
	blocks := generateBlocks(t, 4, txs)

	testTable := []struct {
		name         string
		batch        func() storage.Batch
		expectedGaps []uint64
	}{
		{
			"block save fails",
			func() storage.Batch {
				return &mock.WriteBatch{
					SetBlockFn: func(*types.Block) error { return errors.New("amino incompatible") },
				}
			},
			[]uint64{3},
		},
		{
			"tx save fails",
			func() storage.Batch {
				return &mock.WriteBatch{
					SetTxFn: func(*types.TxResult) error { return errors.New("disk full") },
				}
			},
			[]uint64{3},
		},
		{
			"everything saves",
			func() storage.Batch { return &mock.WriteBatch{} },
			[]uint64{},
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			storageMock := &mock.Storage{GetWriteBatchFn: testCase.batch}

			f := New(storageMock, &mockClient{}, &mockEvents{})

			s := &slot{
				chunk: &chunk{
					blocks:  []*types.Block{blocks[3]},
					results: [][]*types.TxResult{{{Height: 3}, {Height: 3}}},
				},
				chunkRange: chunkRange{from: 3, to: 3},
			}

			require.NoError(t, f.writeSlot(s))
			assert.Equal(t, testCase.expectedGaps, f.gaps.snapshot())
		})
	}
}

func TestDrainGaps(t *testing.T) {
	t.Parallel()

	const txCount = 3

	txs := generateTransactions(t, txCount)
	blocks := generateBlocks(t, 6, txs)

	t.Run("backfills queued heights without advancing the latest height", func(t *testing.T) {
		t.Parallel()

		var (
			mu          sync.Mutex
			savedBlocks []*types.Block
			latestSet   bool
		)

		storageMock := &mock.Storage{
			GetWriteBatchFn: func() storage.Batch {
				return &mock.WriteBatch{
					SetBlockFn: func(b *types.Block) error {
						mu.Lock()
						defer mu.Unlock()

						savedBlocks = append(savedBlocks, b)

						return nil
					},
					SetLatestHeightFn: func(uint64) error {
						latestSet = true

						return nil
					},
				}
			},
		}

		client := &mockClient{
			createBatchFn: erroringBatch,
			getBlockFn: func(num uint64) (*core_types.ResultBlock, error) {
				return &core_types.ResultBlock{Block: blocks[num]}, nil
			},
			getBlockResultsFn: func(num uint64) (*core_types.ResultBlockResults, error) {
				return mockBlockResults(int64(num), txCount), nil
			},
		}

		f := New(storageMock, client, &mockEvents{}, WithChunkFetchRetry(2, time.Millisecond))
		f.gaps.add(2, 4)

		f.drainGaps(context.Background())

		mu.Lock()
		defer mu.Unlock()

		require.Len(t, savedBlocks, 2)
		assert.Equal(t, 0, f.gaps.len(), "queue should be drained")
		assert.False(t, latestSet, "backfill must not advance the latest height")
	})

	t.Run("keeps a height queued when it still cannot be fetched", func(t *testing.T) {
		t.Parallel()

		client := &mockClient{
			createBatchFn: erroringBatch,
			getBlockFn: func(uint64) (*core_types.ResultBlock, error) {
				return nil, errors.New("still down")
			},
		}

		f := New(&mock.Storage{}, client, &mockEvents{}, WithChunkFetchRetry(1, time.Millisecond))
		f.gaps.add(7)

		f.drainGaps(context.Background())

		assert.Equal(t, []uint64{7}, f.gaps.snapshot(), "unfetchable height must stay queued")
	})

	t.Run("keeps a height queued when the fetched block fails to save", func(t *testing.T) {
		t.Parallel()

		storageMock := &mock.Storage{
			GetWriteBatchFn: func() storage.Batch {
				return &mock.WriteBatch{
					SetBlockFn: func(*types.Block) error { return errors.New("amino incompatible") },
				}
			},
		}

		client := &mockClient{
			createBatchFn: erroringBatch,
			getBlockFn: func(num uint64) (*core_types.ResultBlock, error) {
				return &core_types.ResultBlock{Block: blocks[num]}, nil
			},
			getBlockResultsFn: func(num uint64) (*core_types.ResultBlockResults, error) {
				return mockBlockResults(int64(num), txCount), nil
			},
		}

		f := New(storageMock, client, &mockEvents{}, WithChunkFetchRetry(1, time.Millisecond))
		f.gaps.add(5)

		f.drainGaps(context.Background())

		assert.Equal(t, []uint64{5}, f.gaps.snapshot(), "unsavable height must stay queued")
	})
}

func TestRunBackfiller_AuditsThenBackfills(t *testing.T) {
	t.Parallel()

	const txCount = 1

	txs := generateTransactions(t, txCount)
	blocks := generateBlocks(t, 4, txs)

	// Stored: 0, 2 (missing 1), latest 2
	stored := blocksAtHeights(0, 2)

	var (
		mu    sync.Mutex
		saved []*types.Block
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	storageMock := &mock.Storage{
		GetLatestSavedHeightFn: func() (uint64, error) { return 2, nil },
		BlockIteratorFn: func(_, _ uint64) (storage.Iterator[*types.Block], error) {
			return mock.NewSliceBlockIterator(stored), nil
		},
		GetWriteBatchFn: func() storage.Batch {
			return &mock.WriteBatch{
				SetBlockFn: func(b *types.Block) error {
					mu.Lock()
					defer mu.Unlock()

					saved = append(saved, b)

					cancel() // stop the backfiller once the gap is filled

					return nil
				},
			}
		},
	}

	client := &mockClient{
		createBatchFn: erroringBatch,
		getBlockFn: func(num uint64) (*core_types.ResultBlock, error) {
			return &core_types.ResultBlock{Block: blocks[num]}, nil
		},
		getBlockResultsFn: func(num uint64) (*core_types.ResultBlockResults, error) {
			return mockBlockResults(int64(num), txCount), nil
		},
	}

	f := New(
		storageMock,
		client,
		&mockEvents{},
		WithBackfillInterval(time.Hour), // ticker must not fire before ctx is cancelled
		WithStartupAudit(true),
		WithChunkFetchRetry(2, time.Millisecond),
	)

	f.runBackfiller(ctx)

	mu.Lock()
	defer mu.Unlock()

	require.Len(t, saved, 1)
	assert.EqualValues(t, 1, saved[0].Height)
}
