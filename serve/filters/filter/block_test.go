package filter

import (
	"testing"

	"github.com/gnolang/tx-indexer/serve/filters/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockFilter_GetChanges(t *testing.T) {
	t.Parallel()

	// Generate dummy hashes
	hashes := [][]byte{
		[]byte("hash 1"),
		[]byte("hash 2"),
		[]byte("hash 3"),
	}

	// Create a new block filter
	f := NewBlockFilter()

	// Make sure the filter is of a correct type
	assert.Equal(t, BlockFilterType, f.GetType())

	// Update the block filter with dummy blocks
	for _, hash := range hashes {
		hash := hash

		f.UpdateWithBlock(&mocks.MockBlock{
			HashFn: func() []byte {
				return hash
			},
		})
	}

	// Get changes
	changesRaw := f.GetChanges()

	changes, ok := changesRaw.([][]byte)
	require.True(t, ok)

	// Make sure the hashes match
	assert.Equal(t, hashes, changes)
}
