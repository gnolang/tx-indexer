package filter

import (
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	// abci "github.com/gnolang/gno/tm2/pkg/bft/abci/types"
)

type TxFilter struct {
	*baseFilter
	txr			*types.TxResult
}

func NewTxFilter() *TxFilter {
	return &TxFilter{
		baseFilter: newBaseFilter(TxFilterType),
	}
}

func (tf *TxFilter) GetHash() []byte {
	tf.Lock()
	defer tf.Unlock()

	if tf.txr == nil {
		return nil
	}

	tx := tf.txr.Tx
	if tx == nil {
		return nil
	}

	return tx.Hash()
}

func (tf *TxFilter) UpdateWithTx(txr *types.TxResult) {
	tf.Lock()
	defer tf.Unlock()

	tf.txr = txr
}