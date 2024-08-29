package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBaseFilter_GetType(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name       string
		filterType Type
	}{
		{
			"Block filter",
			BlockFilterType,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			f := newBaseFilter(testCase.filterType)

			assert.Equal(t, testCase.filterType, f.GetType())
			assert.Nil(t, f.GetChanges())
		})
	}
}

func TestBaseFilter_LastUsed(t *testing.T) {
	t.Parallel()

	f := newBaseFilter(BlockFilterType)
	lastUsed := f.GetLastUsed()

	// Update the time it was last used
	f.UpdateLastUsed()

	// Make sure the last used time changed
	assert.True(t, lastUsed.Before(f.GetLastUsed()))
}
