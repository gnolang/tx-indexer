package storage

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"testing"

	"github.com/gnolang/gno/gno.land/pkg/gnoland"
	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/gno/tm2/pkg/crypto"
	"github.com/gnolang/gno/tm2/pkg/std"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/gnolang/tx-indexer/internal/contexttest"
	"github.com/gnolang/tx-indexer/internal/tally"
	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
)

func TestStorage_New(t *testing.T) {
	t.Parallel()

	s, err := NewPebble(t.TempDir(), zap.NewNop())
	require.NotNil(t, s)

	assert.NoError(t, err)
	assert.NoError(t, s.Close())
}

func TestStorage_LatestHeight(t *testing.T) {
	t.Parallel()

	s, err := NewPebble(t.TempDir(), zap.NewNop())
	require.NoError(t, err)

	defer func() {
		assert.NoError(t, s.Close())
	}()

	// Make sure no latest height exists
	latest, err := s.GetLatestHeight()
	require.ErrorIs(t, err, storageErrors.ErrNotFound)
	require.EqualValues(t, 0, latest)

	// Save the latest height and grab it
	for i := uint64(0); i < 100; i++ {
		b := s.WriteBatch()

		require.NoError(t, b.SetLatestHeight(i))
		require.NoError(t, b.Commit())

		latest, err = s.GetLatestHeight()

		assert.NoError(t, err)
		assert.EqualValues(t, i, latest)
	}
}

func TestStorage_GenesisChainID(t *testing.T) {
	t.Parallel()

	s, err := NewPebble(t.TempDir(), zap.NewNop())
	require.NoError(t, err)

	defer func() {
		assert.NoError(t, s.Close())
	}()

	// The chain ID is written last, so its absence is what tells the bootstrap
	// there is work to do.
	chainID, err := s.GetGenesisChainID()
	require.ErrorIs(t, err, storageErrors.ErrNotFound)
	require.Empty(t, chainID)

	b := s.WriteBatch()
	require.NoError(t, b.SetGenesisChainID("test-chain"))
	require.NoError(t, b.Commit())

	chainID, err = s.GetGenesisChainID()
	require.NoError(t, err)
	require.Equal(t, "test-chain", chainID)
}

func TestStorage_GenesisBalances(t *testing.T) {
	t.Parallel()

	s, err := NewPebble(t.TempDir(), zap.NewNop())
	require.NoError(t, err)

	defer func() {
		assert.NoError(t, s.Close())
	}()

	balances, err := s.GetGenesisBalances()
	require.ErrorIs(t, err, storageErrors.ErrNotFound)
	require.Nil(t, balances)

	addr := crypto.MustAddressFromString("g1u7y667z64x2h7vc6fmpcprgey4ck233jaww9zq")

	saved := []gnoland.Balance{
		// A plain row and a vesting row: Vesting is a pointer field, so this
		// also pins down that amino round-trips the schedule.
		{
			Address: addr,
			Amount:  std.MustParseCoins("1000ugnot"),
		},
		{
			Address: addr,
			Amount:  std.MustParseCoins("2000ugnot"),
			Vesting: &std.VestingSchedule{
				OriginalVesting: std.MustParseCoins("2000ugnot"),
				StartTime:       100,
				EndTime:         200,
			},
		},
	}

	b := s.WriteBatch()
	require.NoError(t, b.SetGenesisBalances(saved))
	require.NoError(t, b.Commit())

	balances, err = s.GetGenesisBalances()
	require.NoError(t, err)
	require.Equal(t, saved, balances)
}

func TestStorage_Block(t *testing.T) {
	t.Parallel()

	s, err := NewPebble(t.TempDir(), zap.NewNop())
	require.NoError(t, err)

	defer func() {
		assert.NoError(t, s.Close())
	}()

	blocks := generateRandomBlocks(t, 100)

	// Save the blocks and fetch them
	b := s.WriteBatch()
	for _, block := range blocks {
		assert.NoError(t, b.SetBlock(block))
	}

	require.NoError(t, b.Commit())

	for _, block := range blocks {
		savedBlock, err := s.GetBlock(t.Context(), uint64(block.Height))
		require.NoError(t, err)
		assert.Equal(t, block, savedBlock)
	}
}

func TestStorage_Tx(t *testing.T) {
	t.Parallel()

	s, err := NewPebble(t.TempDir(), zap.NewNop())
	require.NoError(t, err)

	defer func() {
		assert.NoError(t, s.Close())
	}()

	txs := generateRandomTxs(t, 100)

	wb := s.WriteBatch()

	// Save the txs and fetch them
	for _, tx := range txs {
		assert.NoError(t, wb.SetTx(tx))
	}

	require.NoError(t, wb.Commit())

	for _, tx := range txs {
		savedTx, err := s.GetTx(t.Context(), uint64(tx.Height), tx.Index)
		require.NoError(t, err)
		assert.Equal(t, tx, savedTx)
	}
}

func TestStorageIters(t *testing.T) {
	t.Parallel()

	s, err := NewPebble(t.TempDir(), zap.NewNop())
	require.NoError(t, err)

	txs := generateRandomTxs(t, 100)
	blocks := generateRandomBlocks(t, 100)

	wb := s.WriteBatch()

	// Save the txs and fetch them
	for i, tx := range txs {
		assert.NoError(t, wb.SetTx(tx))
		assert.NoError(t, wb.SetBlock(blocks[i]))
	}

	require.NoError(t, wb.Commit())

	it, err := s.TxIterator(0, 0, 0, 3)
	require.NoError(t, err)

	txCount := 0

	for {
		if !it.Next(t.Context()) {
			require.NoError(t, it.Error())

			break
		}

		_, err2 := it.Value()
		require.NoError(t, err2)
		require.NoError(t, it.Error())

		txCount++
	}

	require.Equal(t, 4, txCount) // txs 0, 1, 2 AND 3

	defer require.NoError(t, it.Close())

	it2, err := s.BlockIterator(0, 2)
	require.NoError(t, err)

	blockCount := 0

	for {
		if !it2.Next(t.Context()) {
			require.NoError(t, it2.Error())

			break
		}

		_, err2 := it2.Value()
		require.NoError(t, err2)

		blockCount++
	}

	require.Equal(t, 3, blockCount) // blocks 0, 1, AND 2

	defer require.NoError(t, it2.Close())

	it, err = s.TxIterator(0, 0, 20000, 30000)
	require.NoError(t, err)

	txCount = 0

	for {
		if !it.Next(t.Context()) {
			require.NoError(t, it.Error())

			break
		}

		_, err = it.Value()
		require.NoError(t, err)
		require.NoError(t, it.Error())

		txCount++
	}

	require.Equal(t, 0, txCount)

	// corner case: get all transactions from genesis
	it, err = s.TxIterator(0, 0, 0, 0)
	require.NoError(t, err)

	txCount = 0

	for {
		if !it.Next(t.Context()) {
			require.NoError(t, it.Error())

			break
		}

		_, err := it.Value()
		require.NoError(t, err)
		require.NoError(t, it.Error())

		txCount++
	}

	require.Equal(t, 100, txCount)
}

// The transaction iterators yield the rows of their index window in key
// order: by height then index, the reverse iterator the other way round
func TestStorage_TxIteratorsWalkInKeyOrder(t *testing.T) {
	t.Parallel()

	s, err := NewPebble(t.TempDir(), zap.NewNop())
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, s.Close())
	})

	wb := s.WriteBatch()

	for height := int64(1); height <= 3; height++ {
		for index := range 3 {
			require.NoError(t, wb.SetTx(&types.TxResult{
				Height: height,
				Index:  uint32(index),
				Tx:     types.Tx{byte(height), byte(index)},
			}))
		}
	}

	require.NoError(t, wb.Commit())

	// The rows at index 1 and 2, outside index 0
	ascending := []string{"1/1", "1/2", "2/1", "2/2", "3/1", "3/2"}
	descending := []string{"3/2", "3/1", "2/2", "2/1", "1/2", "1/1"}

	testTable := []struct {
		open func() (Iterator[*types.TxResult], error)
		name string
		want []string
	}{
		{
			func() (Iterator[*types.TxResult], error) { return s.TxIterator(0, 0, 1, 0) },
			"ascending",
			ascending,
		},
		{
			func() (Iterator[*types.TxResult], error) { return s.TxReverseIterator(0, 0, 1, 0) },
			"descending",
			descending,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			it, err := testCase.open()
			require.NoError(t, err)

			t.Cleanup(func() {
				assert.NoError(t, it.Close())
			})

			var walked []string

			for it.Next(t.Context()) {
				tx, err := it.Value()
				require.NoError(t, err)

				walked = append(walked, fmt.Sprintf("%d/%d", tx.Height, tx.Index))
			}

			require.NoError(t, it.Error())
			assert.Equal(t, testCase.want, walked)
		})
	}
}

// generateRandomBlocks generates dummy blocks
func generateRandomBlocks(t *testing.T, count int) []*types.Block {
	t.Helper()

	blocks := make([]*types.Block, count)

	for i := 0; i < count; i++ {
		blocks[i] = &types.Block{
			Header: types.Header{
				Height: int64(i),
			},
			Data: types.Data{},
		}
	}

	return blocks
}

// generateRandomTxs generates dummy transactions
func generateRandomTxs(t *testing.T, count int) []*types.TxResult {
	t.Helper()

	txs := make([]*types.TxResult, count)

	for i := 0; i < count; i++ {
		tx := &std.Tx{
			Fee:  std.Fee{},
			Memo: fmt.Sprintf("tx %d", i),
		}

		encodedTx, err := amino.Marshal(tx)
		require.NoError(t, err)

		txs[i] = &types.TxResult{
			Height: 0,
			Index:  uint32(i),
			Tx:     encodedTx,
		}
	}

	return txs
}

// errCountdown is the error the walk test's context ends with
var errCountdown = errors.New("countdown over")

// A transaction iterator walks past every key outside its index window before
// it yields a row, so a narrow window over a long range can read the whole
// chain inside one Next. The walk stops at the first key after its context
// ends.
func TestStorage_TxIteratorWalkStopsWithItsContext(t *testing.T) {
	t.Parallel()

	const (
		heights = 1_000
		budget  = 10
	)

	s, err := NewPebble(t.TempDir(), zap.NewNop())
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, s.Close())
	})

	// One transaction per height, all at index 0: outside a window from 5
	wb := s.WriteBatch()

	for height := int64(1); height <= heights; height++ {
		require.NoError(t, wb.SetTx(&types.TxResult{Height: height, Tx: types.Tx{byte(height), byte(height >> 8)}}))
	}

	require.NoError(t, wb.Commit())

	testTable := []struct {
		open func() (Iterator[*types.TxResult], error)
		name string
	}{
		{
			func() (Iterator[*types.TxResult], error) { return s.TxIterator(0, 0, 5, 0) },
			"ascending",
		},
		{
			func() (Iterator[*types.TxResult], error) { return s.TxReverseIterator(0, 0, 5, 0) },
			"descending",
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			it, err := testCase.open()
			require.NoError(t, err)

			t.Cleanup(func() {
				assert.NoError(t, it.Close())
			})

			ctx := &contexttest.Countdown{
				Context:  context.Background(),
				EndsWith: errCountdown,
				Budget:   budget,
			}

			require.False(t, it.Next(ctx))
			require.ErrorIs(t, it.Error(), errCountdown)

			// One check per key until the context ends, plus the ask for its
			// cause: the walk went no further than budget keys
			assert.LessOrEqual(t, ctx.Asked, budget+2, "the walk ran on after its context ended")
		})
	}
}

// drain walks it to its end and returns the number of rows it yielded
func drain[T any](t *testing.T, ctx context.Context, it Iterator[T]) int {
	t.Helper()

	t.Cleanup(func() {
		assert.NoError(t, it.Close())
	})

	yielded := 0

	for it.Next(ctx) {
		yielded++
	}

	require.NoError(t, it.Error())

	return yielded
}

// Every key an iterator walks counts as a row scanned in the tally its context
// carries, the keys a transaction iterator passes over outside its index
// window included: they cost a read whether or not a row is yielded
func TestStorage_IteratorsTallyTheKeysTheyWalk(t *testing.T) {
	t.Parallel()

	const (
		heights      = 10
		txsPerHeight = 3
	)

	s, err := NewPebble(t.TempDir(), zap.NewNop())
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, s.Close())
	})

	wb := s.WriteBatch()

	for height := int64(1); height <= heights; height++ {
		require.NoError(t, wb.SetBlock(&types.Block{Header: types.Header{Height: height}}))

		for index := range txsPerHeight {
			require.NoError(t, wb.SetTx(&types.TxResult{
				Height: height,
				Index:  uint32(index),
				Tx:     types.Tx{byte(height), byte(index)},
			}))
		}
	}

	require.NoError(t, wb.Commit())

	testTable := []struct {
		walk        func(t *testing.T, ctx context.Context) int
		name        string
		wantYielded int
		wantScanned int64
	}{
		{
			func(t *testing.T, ctx context.Context) int {
				t.Helper()

				it, err := s.TxIterator(0, 0, 0, 0)
				require.NoError(t, err)

				return drain(t, ctx, it)
			},
			"txs",
			heights * txsPerHeight,
			heights * txsPerHeight,
		},
		{
			func(t *testing.T, ctx context.Context) int {
				t.Helper()

				it, err := s.TxIterator(0, 0, 2, 0)
				require.NoError(t, err)

				return drain(t, ctx, it)
			},
			"txs in an index window",
			heights,
			heights * txsPerHeight,
		},
		{
			func(t *testing.T, ctx context.Context) int {
				t.Helper()

				it, err := s.TxReverseIterator(0, 0, 2, 0)
				require.NoError(t, err)

				return drain(t, ctx, it)
			},
			"txs in an index window, descending",
			heights,
			heights * txsPerHeight,
		},
		{
			func(t *testing.T, ctx context.Context) int {
				t.Helper()

				it, err := s.BlockIterator(0, 0)
				require.NoError(t, err)

				return drain(t, ctx, it)
			},
			"blocks",
			heights,
			heights,
		},
		{
			func(t *testing.T, ctx context.Context) int {
				t.Helper()

				it, err := s.BlockIterator(3, 5)
				require.NoError(t, err)

				return drain(t, ctx, it)
			},
			"blocks in a height range",
			3,
			3,
		},
		{
			func(t *testing.T, ctx context.Context) int {
				t.Helper()

				it, err := s.BlockReverseIterator(0, 0)
				require.NoError(t, err)

				return drain(t, ctx, it)
			},
			"blocks, descending",
			heights,
			heights,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rows := &tally.Rows{}

			yielded := testCase.walk(t, tally.NewContext(context.Background(), rows))

			assert.Equal(t, testCase.wantYielded, yielded)
			assert.Equal(t, testCase.wantScanned, rows.Scanned())
		})
	}
}

// Every point read counts as one row scanned in the tally its context
// carries, whether it finds its row or not: a miss reads the storage all the
// same
func TestStorage_PointReadsTallyOneRowEach(t *testing.T) {
	t.Parallel()

	s, err := NewPebble(t.TempDir(), zap.NewNop())
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, s.Close())
	})

	stored := types.Tx{1}

	wb := s.WriteBatch()
	require.NoError(t, wb.SetBlock(&types.Block{Header: types.Header{Height: 1}}))
	require.NoError(t, wb.SetTx(&types.TxResult{Height: 1, Tx: stored}))
	require.NoError(t, wb.Commit())

	hash := base64.StdEncoding.EncodeToString(stored.Hash())

	testTable := []struct {
		read    func(ctx context.Context) error
		wantErr error
		name    string
	}{
		{
			func(ctx context.Context) error {
				_, err := s.GetBlock(ctx, 1)

				return err
			},
			nil,
			"a block found",
		},
		{
			func(ctx context.Context) error {
				_, err := s.GetBlock(ctx, 9)

				return err
			},
			storageErrors.ErrNotFound,
			"a block missing",
		},
		{
			func(ctx context.Context) error {
				_, err := s.GetTx(ctx, 1, 0)

				return err
			},
			nil,
			"a transaction found",
		},
		{
			func(ctx context.Context) error {
				_, err := s.GetTx(ctx, 9, 0)

				return err
			},
			storageErrors.ErrNotFound,
			"a transaction missing",
		},
		{
			func(ctx context.Context) error {
				_, err := s.GetTxByHash(ctx, hash)

				return err
			},
			nil,
			"a hash found",
		},
		{
			func(ctx context.Context) error {
				_, err := s.GetTxByHash(ctx, "unknown")

				return err
			},
			storageErrors.ErrNotFound,
			"a hash missing",
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rows := &tally.Rows{}

			err := testCase.read(tally.NewContext(context.Background(), rows))
			if testCase.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, testCase.wantErr)
			}

			assert.Equal(t, int64(1), rows.Scanned())
		})
	}
}

// pebble's own messages are written through the indexer's logger: its
// information at info, its background errors at error, and its fatal errors
// through the logger's fatal
func TestPebble_LogsThroughTheLogger(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.InfoLevel)
	options := pebbleOptions(zap.New(core, zap.WithFatalHook(zapcore.WriteThenPanic)))

	options.Logger.Infof("compacted %d tables", 3)
	options.EventListener.BackgroundError(errors.New("the disk failed"))

	assert.Panics(t, func() {
		options.Logger.Fatalf("corrupted %s", "MANIFEST")
	})

	entries := logs.All()
	require.Len(t, entries, 3)

	assert.Equal(t, zap.InfoLevel, entries[0].Level)
	assert.Equal(t, "compacted 3 tables", entries[0].Message)

	assert.Equal(t, zap.ErrorLevel, entries[1].Level)
	assert.Equal(t, "the disk failed", entries[1].ContextMap()["error"])

	assert.Equal(t, zap.FatalLevel, entries[2].Level)
	assert.Equal(t, "corrupted MANIFEST", entries[2].Message)
}
