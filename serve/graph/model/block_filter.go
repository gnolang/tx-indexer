package model

import (
	"time"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

type BlockFilter struct {
	// Minimum block height from which to start fetching Blocks, inclusive.
	// Aids in scoping the search to recent Blocks.
	FromHeight *int `json:"fromHeight,omitempty"`
	// Maximum block height up to which Blocks should be fetched, exclusive.
	// Helps in limiting the search to older Blocks.
	ToHeight *int `json:"toHeight,omitempty"`
	// Minimum block create time from which to start fetching Blocks, inclusive.
	// Aids in scoping the search to recent Blocks.
	FromTime *time.Time `json:"fromTime,omitempty"`
	// Maximum block create time up to which Blocks should be fetched, exclusive.
	// Helps in limiting the search to older Blocks.
	ToTime *time.Time `json:"toTime,omitempty"`
}

func (filter *BlockFilter) GetFromHeight() uint64 {
	return uint64(Deref(filter.FromHeight))
}

func (filter *BlockFilter) GetToHeight() uint64 {
	return uint64(Deref(filter.ToHeight))
}

func (filter *BlockFilter) GetFromTime() time.Time {
	return Deref(filter.FromTime)
}

func (filter *BlockFilter) GetToTime() time.Time {
	return Deref(filter.ToTime)
}

func (filter *BlockFilter) FilterBy(block *types.Block) bool {
	return filter.filterByBlockTime(block.Time)
}

func (filter *BlockFilter) filterByBlockTime(blockTime time.Time) bool {
	fromTime := filter.GetFromTime()
	toTIme := filter.GetToTime()

	return (blockTime.After(fromTime) || blockTime.Equal(fromTime)) && (toTIme.IsZero() || blockTime.Before(toTIme))
}
