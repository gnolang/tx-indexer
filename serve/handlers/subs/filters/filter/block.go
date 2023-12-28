package filter

import "github.com/gnolang/tx-indexer/types"

// BlockFilter type of filter for querying blocks
type BlockFilter struct {
	*baseFilter

	blockHashes [][]byte // TODO keep as strings in 0x format?
}

// NewBlockFilter creates new block filter object
func NewBlockFilter() *BlockFilter {
	return &BlockFilter{
		baseFilter:  newBaseFilter(BlockFilterType),
		blockHashes: make([][]byte, 0),
	}
}

// GetChanges returns all new blocks from last query
func (b *BlockFilter) GetChanges() any {
	b.RLock()
	defer b.RUnlock()

	// Get hashes
	hashes := b.blockHashes

	// Empty hashes
	b.blockHashes = b.blockHashes[:0]

	return hashes
}

func (b *BlockFilter) UpdateWithBlock(block types.Block) {
	b.Lock()
	defer b.Unlock()

	// Fetch block hash
	hash := block.Hash()

	// Add hash into block hash array
	b.blockHashes = append(b.blockHashes, hash)
}
