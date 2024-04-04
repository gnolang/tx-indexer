package graph

import (
	"time"

	"github.com/gnolang/tx-indexer/serve/graph/model"
)

// `FilteredBlockBy` checks for conditions in BlockTime.
// By default, the condition is only checked if the input parameter exists.
func FilteredBlockBy(block *model.Block, filter model.BlockFilter) bool {
	if block == nil {
		return false
	}

	return filteredBlockByBlockTime(block, filter.FromTime, filter.ToTime)
}

// `filteredBlockByBlockTime` checks block based on block time.
func filteredBlockByBlockTime(block *model.Block, filterFromTime, filterToTime *time.Time) bool {
	fromTime := deref(filterFromTime)
	toTime := deref(filterToTime)

	if filterToTime == nil {
		toTime = time.Now()
	}

	blockTime := block.Time()

	return (blockTime.After(fromTime) || blockTime.Equal(fromTime)) && (toTime.IsZero() || blockTime.Before(toTime))
}
