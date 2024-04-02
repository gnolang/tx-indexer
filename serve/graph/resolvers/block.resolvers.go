package resolvers

import (
	"time"

	"github.com/gnolang/tx-indexer/serve/graph/model"
)

// The resolver for handling the block model.
type BlockResolver struct {
	block *model.Block
}

func NewBlockResolver(block *model.Block) *BlockResolver {
	return &BlockResolver{block: block}
}

func (r *BlockResolver) GetBlock() *model.Block {
	return r.block
}

func (r *BlockResolver) FilteredBy(filter model.BlockFilter) bool {
	if r.block == nil {
		return false
	}

	return r.filteredByBlockTime(filter.FromTime, filter.ToTime)
}

func (r *BlockResolver) filteredByBlockTime(filterFromTime, filterToTime *time.Time) bool {
	fromTime := deref(filterFromTime)
	toTime := deref(filterToTime)
	blockTime := r.block.Time()

	return (blockTime.After(fromTime) || blockTime.Equal(fromTime)) && (toTime.IsZero() || blockTime.Before(toTime))
}
