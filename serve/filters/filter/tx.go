package filter

import (
	"math"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

type RangeFilterOption struct {
	Min *int64 `json:"min,omitempty"`
	Max *int64 `json:"max,omitempty"`
}

type TxFilterOption struct {
	GasUsed   *RangeFilterOption `json:"gasUsed,omitempty"`
	GasWanted *RangeFilterOption `json:"gasWanted,omitempty"`
	GasLimit  *RangeFilterOption `json:"gasLimit,omitempty"`
}

// TxFilter holds a slice of transaction results.
// It provides methods to manipulate and query the transactions.
type TxFilter struct {
	opts TxFilterOption
	*baseFilter
	txs []types.TxResult
}

// NewTxFilter creates a new TxFilter object.
func NewTxFilter(opts TxFilterOption) *TxFilter {
	return &TxFilter{
		baseFilter: newBaseFilter(TxFilterType),
		txs:        make([]types.TxResult, 0),
		opts:       opts,
	}
}

// GetChanges returns all new transactions from the last query
func (tf *TxFilter) GetChanges() any {
	return tf.getTxChanges()
}

func (tf *TxFilter) UpdateWith(data any) {
	tx, ok := data.(*types.TxResult)
	if !ok {
		return
	}

	if tf.checkFilterOptions(tx) {
		tf.updateWithTx(tx)
	}
}

// GetHashes iterates over all transactions in the filter and returns their hashes.
func (tf *TxFilter) GetHashes() [][]byte {
	tf.Lock()
	defer tf.Unlock()

	hashes := make([][]byte, 0, len(tf.txs))

	for _, txr := range tf.txs {
		var hash []byte

		if txr.Tx != nil {
			hash = txr.Tx.Hash()
		}

		hashes = append(hashes, hash)
	}

	return hashes
}

// `checkFilterOptions` checks the conditions of the options in the filter.
func (tf *TxFilter) checkFilterOptions(tx *types.TxResult) bool {
	if !filteredByRangeFilterOption(tx.Response.GasUsed, tf.opts.GasUsed) {
		return false
	}

	if !filteredByRangeFilterOption(tx.Response.GasWanted, tf.opts.GasWanted) {
		return false
	}

	// GasLimit compares GasUsed.
	if !filteredByRangeFilterOption(tx.Response.GasUsed, tf.opts.GasLimit) {
		return false
	}

	return true
}

// `filteredByRangeFilterOption` checks if the number is in a range.
func filteredByRangeFilterOption(value int64, rangeFilterOption *RangeFilterOption) bool {
	if rangeFilterOption == nil {
		return true
	}

	min := int64(0)
	if rangeFilterOption.Min != nil {
		min = *rangeFilterOption.Min
	}

	max := int64(math.MaxInt64)
	if rangeFilterOption.Max != nil {
		max = *rangeFilterOption.Max
	}

	return value >= min && value <= max
}

// getTxChanges returns all new transactions from the last query
func (tf *TxFilter) getTxChanges() []types.TxResult {
	tf.Lock()
	defer tf.Unlock()

	// Get newTxs
	newTxs := make([]types.TxResult, len(tf.txs))
	copy(newTxs, tf.txs)

	// Empty headers
	tf.txs = tf.txs[:0]

	return newTxs
}

func (tf *TxFilter) updateWithTx(tx *types.TxResult) {
	tf.Lock()
	defer tf.Unlock()

	tf.txs = append(tf.txs, *tx)
}
