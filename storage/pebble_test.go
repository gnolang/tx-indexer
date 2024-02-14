package storage

import (
	"fmt"
	"testing"

	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/gno/tm2/pkg/std"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
)

func TestStorage_New(t *testing.T) {
	t.Parallel()

	s, err := NewPebble(t.TempDir())
	require.NotNil(t, s)

	assert.NoError(t, err)
	assert.NoError(t, s.Close())
}

func TestStorage_LatestHeight(t *testing.T) {
	t.Parallel()

	s, err := NewPebble(t.TempDir())
	require.NoError(t, err)

	defer func() {
		assert.NoError(t, s.Close())
	}()

	// Make sure no latest height exists
	latest, err := s.GetLatestHeight()
	require.ErrorIs(t, err, storageErrors.ErrNotFound)
	require.EqualValues(t, 0, latest)

	// Save the latest height and grab it
	for i := int64(0); i < 100; i++ {
		b := s.WriteBatch()

		require.NoError(t, b.SetLatestHeight(i))
		require.NoError(t, b.Commit())

		latest, err = s.GetLatestHeight()

		assert.NoError(t, err)
		assert.EqualValues(t, i, latest)
	}
}

func TestStorage_Block(t *testing.T) {
	t.Parallel()

	s, err := NewPebble(t.TempDir())
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
		savedBlock, err := s.GetBlock(block.Height)
		require.NoError(t, err)
		assert.Equal(t, block, savedBlock)
	}
}

func TestStorage_Tx(t *testing.T) {
	t.Parallel()

	s, err := NewPebble(t.TempDir())
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
		savedTx, err := s.GetTx(tx.Height, tx.Index)
		require.NoError(t, err)
		assert.Equal(t, tx, savedTx)
	}
}

func TestStorageIters(t *testing.T) {
	t.Parallel()

	s, err := NewPebble(t.TempDir())
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
		if !it.Next() {
			require.NoError(t, it.Error())

			break
		}

		_, err2 := it.Value()
		require.NoError(t, err2)
		require.NoError(t, it.Error())

		txCount++
	}

	require.Equal(t, 2, txCount)

	defer require.NoError(t, it.Close())

	it2, err := s.BlockIterator(0, 2)
	require.NoError(t, err)

	blockCount := 0

	for {
		if !it2.Next() {
			require.NoError(t, it2.Error())

			break
		}

		_, err2 := it2.Value()
		require.NoError(t, err2)

		blockCount++
	}

	require.Equal(t, 2, blockCount)

	defer require.NoError(t, it2.Close())

	it, err = s.TxIterator(0, 0, 20000, 30000)
	require.NoError(t, err)

	txCount = 0

	for {
		if !it.Next() {
			require.NoError(t, it.Error())

			break
		}

		_, err := it.Value()
		require.NoError(t, err)
		require.NoError(t, it.Error())

		txCount++
	}

	require.Equal(t, 0, txCount)
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
