package storage

import (
	"fmt"
	"testing"

	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/gno/tm2/pkg/std"
	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorage_New(t *testing.T) {
	t.Parallel()

	s, err := New(t.TempDir())
	require.NotNil(t, s)

	assert.NoError(t, err)
	assert.NoError(t, s.Close())
}

func TestStorage_LatestHeight(t *testing.T) {
	t.Parallel()

	s, err := New(t.TempDir())
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
		require.NoError(t, s.SaveLatestHeight(i))

		latest, err = s.GetLatestHeight()

		assert.NoError(t, err)
		assert.EqualValues(t, i, latest)
	}
}

func TestStorage_Block(t *testing.T) {
	t.Parallel()

	s, err := New(t.TempDir())
	require.NoError(t, err)

	defer func() {
		assert.NoError(t, s.Close())
	}()

	blocks := generateRandomBlocks(t, 100)

	// Save the blocks and fetch them
	for _, block := range blocks {
		assert.NoError(t, s.SaveBlock(block))

		savedBlock, err := s.GetBlock(block.Height)
		require.NoError(t, err)

		assert.Equal(t, block, savedBlock)
	}
}

func TestStorage_Tx(t *testing.T) {
	t.Parallel()

	s, err := New(t.TempDir())
	require.NoError(t, err)

	defer func() {
		assert.NoError(t, s.Close())
	}()

	txs := generateRandomTxs(t, 100)

	// Save the txs and fetch them
	for _, tx := range txs {
		assert.NoError(t, s.SaveTx(tx))

		savedTx, err := s.GetTx(tx.Tx.Hash())
		require.NoError(t, err)

		assert.Equal(t, tx, savedTx)
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
