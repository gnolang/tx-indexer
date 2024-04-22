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
func (b *BlockFilter) GetChanges() []any {
	return b.getBlockChanges()
}

func (b *BlockFilter) UpdateWith(data any) {
	if block, ok := data.(*types.Block); ok {
		b.updateWithBlock(block)
	}
}

// getBlockChanges returns all new block headers from the last query
func (b *BlockFilter) getBlockChanges() []any {
	b.Lock()
	defer b.Unlock()

	// Get hashes
	hashes := make([]any, len(b.blockHeaders))
	for index, blockHeader := range b.blockHeaders {
		hashes[index] = blockHeader
	}

	// Empty headers
	b.blockHeaders = b.blockHeaders[:0]

	return hashes
}

func (b *BlockFilter) updateWithBlock(block *types.Block) {
	b.Lock()
	defer b.Unlock()

	// Add header into block header array
	b.blockHeaders = append(
		b.blockHeaders,
		block.Header,
	)
}
