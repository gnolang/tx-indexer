package filter

import (
	"encoding/base64"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

// BlockFilter type of filter for querying blocks
type BlockFilter struct {
	*baseFilter

	blockHashes []string
}

// NewBlockFilter creates new block filter object
func NewBlockFilter() *BlockFilter {
	return &BlockFilter{
		baseFilter:  newBaseFilter(BlockFilterType),
		blockHashes: make([]string, 0),
	}
}

// GetChanges returns all new block headers from the last query
func (b *BlockFilter) GetChanges() any {
	b.RLock()
	defer b.RUnlock()

	// Get hashes
	hashes := b.blockHashes

	// Empty headers
	b.blockHashes = b.blockHashes[:0]

	return hashes
}

func (b *BlockFilter) UpdateWithBlock(block *types.Block) {
	b.Lock()
	defer b.Unlock()

	// Get the block hash
	hash := block.Hash()

	// Add hash into block hash array
	b.blockHashes = append(b.blockHashes, base64.StdEncoding.EncodeToString(hash))
}
