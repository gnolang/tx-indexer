package filter

import (
	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

// TxFilter holds a slice of transaction results.
// It provides methods to manipulate and query the transactions.
type TxFilter struct {
	*baseFilter
	txrs		[]*types.TxResult
}

// NewTxFilter creates a new TxFilter object.
func NewTxFilter() *TxFilter {
	return &TxFilter{
		baseFilter: newBaseFilter(TxFilterType),
		txrs: make([]*types.TxResult, 0),
	}
}

// GetHashes iterates over all transactions in the filter and returns their hashes.
//
// It appends `nil` to the result slice if the transaction or its content is `nil`.
// This ensures that the length og the returned slice matches the number of transactions in the filter.
func (tf *TxFilter) GetHashes() [][]byte {
	tf.Lock()
	defer tf.Unlock()

	hashes := make([][]byte, 0, len(tf.txrs))
	for _, txr := range tf.txrs {
		if txr == nil || txr.Tx == nil {
			hashes = append(hashes, nil)
			continue
		}
		hashes = append(hashes, txr.Tx.Hash())
	}

	return hashes
}

// GetChanges retrieves and returns all the transactions in the filter.
//
// It also resets the transactions and prepare the filter for new transactions.
func (tf *TxFilter) GetChanges() any {
	tf.Lock()
	defer tf.Unlock()

	changes := make([]*types.TxResult, len(tf.txrs))
	copy(changes, tf.txrs)

	tf.txrs = tf.txrs[:0]	// reset for new transactions

	return changes
}

// UpdateWithTx adds a transaction to the filter.
func (tf *TxFilter) UpdateWithTx(txr *types.TxResult) {
	tf.Lock()
	defer tf.Unlock()

	tf.txrs = append(tf.txrs, txr)
}