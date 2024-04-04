package graph

import (
	"time"

	"github.com/gnolang/tx-indexer/serve/graph/model"
)

func FilteredBlockBy(block *model.Block, filter model.BlockFilter) bool {
	if block == nil {
		return false
	}

	return filteredBlockByBlockTime(block, filter.FromTime, filter.ToTime)
}

func filteredBlockByBlockTime(block *model.Block, filterFromTime, filterToTime *time.Time) bool {
	fromTime := deref(filterFromTime)
	toTime := deref(filterToTime)

	if filterToTime == nil {
		toTime = time.Now()
	}

	blockTime := block.Time()

	return (blockTime.After(fromTime) || blockTime.Equal(fromTime)) && (toTime.IsZero() || blockTime.Before(toTime))
}
