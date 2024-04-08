package filter

import (
	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

type Options struct {
	Height    *int64
	Index     *uint32
	GasUsed   struct{ Min, Max *int64 }
	GasWanted struct{ Min, Max *int64 }
}

// TxFilter holds a slice of transaction results.
// It provides methods to manipulate and query the transactions.
type TxFilter struct {
	*baseFilter
	txs  []*types.TxResult
	opts Options
}

// NewTxFilter creates a new TxFilter object.
func NewTxFilter(opts Options) *TxFilter {
	return &TxFilter{
		baseFilter: newBaseFilter(TxFilterType),
		txs:        make([]*types.TxResult, 0),
		opts:       opts,
	}
}

// GetHashes iterates over all transactions in the filter and returns their hashes.
func (tf *TxFilter) GetHashes() [][]byte {
	tf.Lock()
	defer tf.Unlock()

	hashes := make([][]byte, 0, len(tf.txs))

	for _, txr := range tf.txs {
		var hash []byte

		if txr != nil && txr.Tx != nil {
			hash = txr.Tx.Hash()
		}

		hashes = append(hashes, hash)
	}

	return hashes
}

func (tf *TxFilter) UpdateWithTx(tx *types.TxResult) {
	tf.Lock()
	defer tf.Unlock()

	tf.txs = append(tf.txs, tx)
}

// Apply applies all added conditions to the transactions in the filter.
//
// It returns a slice of `TxResult` that satisfy all the conditions. If no conditions are set,
// it returns all transactions in the filter.
func (tf *TxFilter) Apply() []*types.TxResult {
	tf.Lock()
	defer tf.Unlock()

	return checkOpts(tf.txs, tf.opts)
}

func checkOpts(txs []*types.TxResult, opts Options) []*types.TxResult {
	filtered := make([]*types.TxResult, 0, len(txs))

	for _, tx := range txs {
		if opts.Height != nil && tx.Height != *opts.Height {
			continue
		}

		if opts.Index != nil && tx.Index != *opts.Index {
			continue
		}

		if opts.GasUsed.Max != nil && tx.Response.GasUsed > *opts.GasUsed.Max {
			continue
		}

		if opts.GasUsed.Min != nil && tx.Response.GasUsed < *opts.GasUsed.Min {
			continue
		}

		if opts.GasWanted.Max != nil && tx.Response.GasWanted > *opts.GasWanted.Max {
			continue
		}

		if opts.GasWanted.Min != nil && tx.Response.GasWanted < *opts.GasWanted.Min {
			continue
		}

		filtered = append(filtered, tx)
	}

	return filtered
}
