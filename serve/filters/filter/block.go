package filter

import (
	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

// BlockFilter type of filter for querying blocks
type BlockFilter struct {
	*baseFilter

	blockHeaders []types.Header
}

// NewBlockFilter creates new block filter object
func NewBlockFilter() *BlockFilter {
	return &BlockFilter{
		baseFilter:   newBaseFilter(BlockFilterType),
		blockHeaders: make([]types.Header, 0),
	}
}

// GetChanges returns all new block headers from the last query
func (b *BlockFilter) GetChanges() any {
	b.Lock()
	defer b.Unlock()

	// Get hashes
	hashes := make([]types.Header, len(b.blockHeaders))
	copy(hashes, b.blockHeaders)

	// Empty headers
	b.blockHeaders = b.blockHeaders[:0]

	return hashes
}

func (b *BlockFilter) UpdateWithBlock(block *types.Block) {
	b.Lock()
	defer b.Unlock()

	// Add header into block header array
	b.blockHeaders = append(
		b.blockHeaders,
		block.Header,
	)
}
