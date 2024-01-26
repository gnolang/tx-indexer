package filter

import (
	"sort"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

// filterPriority defines the priority of a filter condition.
// The higher the priority, the earlier the condition is applied.
//
// This concept is borrowed from operator precedence.
type filterPriority int

const (
	HeightPriority filterPriority = iota     // highest priority
	IndexPriority
	GasUsedPriority
	GasWantedPriority						 // lowest priority
)

type condition struct {
	filter    	func(*types.TxResult) bool
	priority  	filterPriority
}

// TxFilter holds a slice of transaction results.
// It provides methods to manipulate and query the transactions.
type TxFilter struct {
	*baseFilter
	// txrs represents the transactions in the filter.
	txrs []*types.TxResult
	// conditions holds the filtering conditions.
	conditions []condition
}

// NewTxFilter creates a new TxFilter object.
func NewTxFilter() *TxFilter {
	return &TxFilter{
		baseFilter: newBaseFilter(TxFilterType),
		txrs:       make([]*types.TxResult, 0),
		conditions: make([]condition, 0),
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

	tf.txrs = tf.txrs[:0] // reset for new transactions

	return changes
}

// UpdateWithTx adds a transaction to the filter.
func (tf *TxFilter) UpdateWithTx(txr *types.TxResult) {
	tf.Lock()
	defer tf.Unlock()

	tf.txrs = append(tf.txrs, txr)
}

// ClearConditions resets the previously set conditions from the filter.
func (tf *TxFilter) ClearConditions() *TxFilter {
	tf.Lock()
	defer tf.Unlock()

	tf.conditions = nil

	return tf
}

// Height sets a filter for the height of the transactions.
//
// It appends a height-based condition to the conditions slice.
func (tf *TxFilter) Height(height int64) *TxFilter {
	cond := condition {
		func(txr *types.TxResult) bool {
			return txr.Height == height
		},
		HeightPriority,
	}
	tf.insertConditionInOrder(cond)

	return tf
}

// Index sets a filter for the index of the transactions.
func (tf *TxFilter) Index(index uint32) *TxFilter {
	cond := condition {
		func(txr *types.TxResult) bool {
			return txr.Index == index
		},
		IndexPriority,
	}
	tf.insertConditionInOrder(cond)

	return tf
}

// GasUsed sets a filter for the gas used by transactions.
func (tf *TxFilter) GasUsed(min, max int64) *TxFilter {
	cond := condition {
		func(txr *types.TxResult) bool {
			return txr.Response.GasUsed >= min && txr.Response.GasUsed <= max
		},
		GasUsedPriority,
	}
	tf.insertConditionInOrder(cond)

	return tf
}

// GasWanted sets a filter for the gas wanted by transactions.
func (tf *TxFilter) GasWanted(min, max int64) *TxFilter {
	cond := condition {
		func(txr *types.TxResult) bool {
			return txr.Response.GasWanted >= min && txr.Response.GasWanted <= max
		},
		GasWantedPriority,
	}
	tf.insertConditionInOrder(cond)

	return tf
}

// Apply applies all added conditions to the transactions in the filter.
//
// It returns a slice of `TxResult` that satisfy all the conditions.
func (tf *TxFilter) Apply() []*types.TxResult {
	tf.Lock()
	defer tf.Unlock()

	if len(tf.conditions) == 0 {
		return tf.txrs
	}

	var filtered []*types.TxResult

	for _, txr := range tf.txrs {
		pass := true

		for _, condition := range tf.conditions {
			if !condition.filter(txr) {
				pass = false

				break
			}
		}

		if pass {
			filtered = append(filtered, txr)
		}
	}

	return filtered
}

// insertConditionInOrder adds a new condition to the conditions slice.
//
// It places the condition at the right position based on its priority,
// ensuring the slice is always ordered without needing to sort it entirely each time.
//
// complexity: O(log n) + O(n)
func (tf *TxFilter) insertConditionInOrder(cond condition) {
	tf.Lock()
	defer tf.Unlock()

	i := sort.Search(len(tf.conditions), func(i int) bool {
		return tf.conditions[i].priority >= cond.priority
	})

	tf.conditions = append(tf.conditions[:i], append([]condition{cond}, tf.conditions[i:]...)...)
}
