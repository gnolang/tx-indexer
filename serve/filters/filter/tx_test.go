package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

func TestGetHash(t *testing.T) {
	t.Parallel()

	txs := []*types.TxResult {
		{ Tx: []byte(`c25dda249cdece9d908cc33adcd16aa05e20290f`) },
		{ Tx: []byte(`71ac9eed6a76a285ae035fe84a251d56ae9485a4`) },
		{ Tx: []byte(`356a192b7913b04c54574d18c28d46e6395428ab`) },
		{ Tx: []byte(`da4b9237bacccdf19c0760cab7aec4a8359010b0`) },
		{ Tx: []byte(`77de68daecd823babbb58edb1c8e14d7106e83bb`) },
		{ Tx: []byte(`1b6453892473a467d07372d45eb05abc2031647a`) },
		{ Tx: []byte(`ac3478d69a3c81fa62e60f5c3696165a4e5e6ac4`) },
		{ Tx: []byte(`c1dfd96eea8cc2b62785275bca38ac261256e278`) },
	}

	// create a new tx filter
	f := NewTxFilter()

	// make sure the filter is of a correct type
	assert.Equal(t, TxFilterType, f.GetType())

	// update the tx filter with dummy txs
	for _, tx := range txs {
		tx := tx

		f.UpdateWithTx(tx)

		// get hash
		hash := f.GetHash()
		assert.Equal(t, tx.Tx.Hash(), hash)
	}

	// change last tx to nil
	f.UpdateWithTx(nil)

	hash := f.GetHash()
	assert.Nil(t, hash)
}