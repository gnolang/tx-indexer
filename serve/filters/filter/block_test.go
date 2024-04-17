package filter

import (
	"testing"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockFilter_GetChanges(t *testing.T) {
	t.Parallel()

	// Generate dummy blocks
	blocks := []*types.Block{
		{
			Header: types.Header{
				Height: 1,
			},
		},
		{
			Header: types.Header{
				Height: 2,
			},
		},
		{
			Header: types.Header{
				Height: 3,
			},
		},
	}

	// Create a new block filter
	f := NewBlockFilter()

	// Make sure the filter is of a correct type
	assert.Equal(t, BlockFilterType, f.GetType())

	// Update the block filter with dummy blocks
	for _, block := range blocks {
		block := block

		f.UpdateWith(block)
	}

	// Get changes
	changesRaw := f.GetChanges()

	changes, ok := changesRaw.([]types.Header)
	require.True(t, ok)

	// Make sure the headers match
	require.Len(t, changes, len(blocks))

	for index, header := range changes {
		assert.Equal(t, blocks[index].Header, header)
	}
}
