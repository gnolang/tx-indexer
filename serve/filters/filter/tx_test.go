package filter

import (
	"testing"

	abci "github.com/gnolang/gno/tm2/pkg/bft/abci/types"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetHashAndChanges(t *testing.T) {
	t.Parallel()

	txs := []*types.TxResult{
		{Tx: []byte(`c25dda249cdece9d908cc33adcd16aa05e20290f`)},
		{Tx: []byte(`71ac9eed6a76a285ae035fe84a251d56ae9485a4`)},
		{Tx: []byte(`356a192b7913b04c54574d18c28d46e6395428ab`)},
		{Tx: []byte(`da4b9237bacccdf19c0760cab7aec4a8359010b0`)},
		{Tx: []byte(`77de68daecd823babbb58edb1c8e14d7106e83bb`)},
		{Tx: []byte(`1b6453892473a467d07372d45eb05abc2031647a`)},
		{Tx: []byte(`ac3478d69a3c81fa62e60f5c3696165a4e5e6ac4`)},
		{Tx: []byte(`c1dfd96eea8cc2b62785275bca38ac261256e278`)},
	}

	// create a new tx filter
	f := NewTxFilter()
	assert.Equal(t, TxFilterType, f.GetType())

	for _, tx := range txs {
		f.UpdateWithTx(tx)
	}

	// get the hashes of the txs
	hashes := f.GetHashes()
	for i, tx := range txs {
		assert.Equal(t, tx.Tx.Hash(), hashes[i])
	}

	// get the chages from the filter
	changes := f.GetChanges().([]*types.TxResult)
	require.Len(t, changes, len(txs))

	for i, tx := range txs {
		assert.Equal(t, tx, changes[i])
	}

	assert.Empty(t, f.txrs)
}

func TestTxFilters(t *testing.T) {
	txs := []*types.TxResult{
		{
			Height: 100,
			Index:  0,
			Tx:     []byte(`sampleTx0`),
			Response: abci.ResponseDeliverTx{
				GasWanted: 1000,
				GasUsed:   900,
				ResponseBase: abci.ResponseBase{
					Data: []byte(`data0`),
				},
			},
		},
		{
			Height: 101,
			Index:  1,
			Tx:     []byte(`sampleTx1`),
			Response: abci.ResponseDeliverTx{
				GasWanted: 1200,
				GasUsed:   1100,
				ResponseBase: abci.ResponseBase{
					Data: []byte(`data1`),
				},
			},
		},
		{
			Height: 102,
			Index:  2,
			Tx:     []byte(`sampleTx2`),
			Response: abci.ResponseDeliverTx{
				GasWanted: 900,
				GasUsed:   800,
				ResponseBase: abci.ResponseBase{
					Data: []byte(`data2`),
				},
			},
		},
		{
			Height: 103,
			Index:  3,
			Tx:     []byte(`sampleTx3`),
			Response: abci.ResponseDeliverTx{
				GasWanted: 1300,
				GasUsed:   1250,
				ResponseBase: abci.ResponseBase{
					Data: []byte(`data3`),
				},
			},
		},
		{
			Height: 104,
			Index:  4,
			Tx:     []byte(`sampleTx4`),
			Response: abci.ResponseDeliverTx{
				GasWanted: 800,
				GasUsed:   700,
				ResponseBase: abci.ResponseBase{
					Data: []byte(`data4`),
				},
			},
		},
	}

	f := NewTxFilter()
	for _, tx := range txs {
		f.UpdateWithTx(tx)
	}

	height := f.Height(100).Apply()
	require.Len(t, height, 1)
	assert.Equal(t, txs[0], height[0])

	f.ClearConditions()

	index := f.Index(1).Apply()
	require.Len(t, index, 1)
	assert.Equal(t, txs[1], index[0])

	f.ClearConditions()

	gasUsed := f.GasUsed(800, 1000).Apply()
	require.Len(t, gasUsed, 2)
	assert.Equal(t, txs[0], gasUsed[0])
	assert.Equal(t, txs[2], gasUsed[1])

	f.ClearConditions()

	gasWanted := f.GasWanted(1000, 1200).Apply()
	require.Len(t, gasWanted, 2)
	assert.Equal(t, txs[0], gasWanted[0])
	assert.Equal(t, txs[1], gasWanted[1])

	f.ClearConditions()

	// query-like method chaining
	query := f.Height(101).Index(1).GasUsed(1000, 1200).Apply()
	require.Len(t, query, 1)
	assert.Equal(t, txs[1], query[0])

	f.ClearConditions()
}
