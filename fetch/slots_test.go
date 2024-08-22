package fetch

import (
	"testing"

	queue "github.com/madz-lab/insertion-queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlots_GetSlot(t *testing.T) {
	t.Parallel()

	ranges := []chunkRange{
		{
			from: 0,
			to:   1,
		},
		{
			from: 2,
			to:   3,
		},
		{
			from: 4,
			to:   5,
		},
	}

	s := &slots{
		make([]queue.Item, 0, len(ranges)),
		len(ranges),
	}

	for _, chunkRange := range ranges {
		s.Push(&slot{
			chunkRange: chunkRange,
		})
	}

	assert.Equal(t, s.Len(), len(ranges))

	for index, chunkRange := range ranges {
		slot := s.getSlot(index)

		assert.Equal(t, chunkRange, slot.chunkRange)
		assert.Nil(t, slot.chunk)
	}
}

func TestSlots_FindGaps(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name string

		existingRanges []chunkRange
		expectedRanges []chunkRange

		start        uint64
		end          uint64
		maxChunkSize int64
	}{
		{
			"no existing ranges",
			[]chunkRange{},
			[]chunkRange{
				{
					from: 1,
					to:   5,
				},
				{
					from: 6,
					to:   10,
				},
			},
			1,
			10,
			5,
		},
		{
			"existing later gaps",
			[]chunkRange{
				{
					from: 1,
					to:   5,
				},
				{
					from: 6,
					to:   10,
				},
			},
			[]chunkRange{
				{
					from: 11,
					to:   15,
				},
			},
			1,
			15,
			5,
		},
		{
			"existing middle gaps",
			[]chunkRange{
				{
					from: 1,
					to:   10,
				},
				{
					from: 20,
					to:   30,
				},
			},
			[]chunkRange{
				{
					from: 11,
					to:   19,
				},
			},
			1,
			30,
			10,
		},
		{
			"existing early gaps",
			[]chunkRange{
				{
					from: 20,
					to:   30,
				},
			},
			[]chunkRange{
				{
					from: 1,
					to:   10,
				},
				{
					from: 11,
					to:   19,
				},
			},
			1,
			30,
			10,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			s := &slots{
				make([]queue.Item, 0, DefaultMaxSlots),
				DefaultMaxSlots,
			}

			for _, chunkRange := range testCase.existingRanges {
				s.Push(&slot{
					chunkRange: chunkRange,
				})
			}

			gaps := s.findGaps(
				testCase.start,
				testCase.end,
				testCase.maxChunkSize,
			)

			assert.Equal(t, testCase.expectedRanges, gaps)
		})
	}
}

func TestSlots_ReserveChunkRanges(t *testing.T) {
	t.Parallel()

	existingRanges := []chunkRange{
		{
			from: 11,
			to:   20,
		},
		{
			from: 31,
			to:   40,
		},
	}

	expectedRanges := []chunkRange{
		{
			from: 1,
			to:   10,
		},
		{
			from: 11,
			to:   20,
		},
		{
			from: 21,
			to:   30,
		},
		{
			from: 31,
			to:   40,
		},
		{
			from: 41,
			to:   50,
		},
	}

	// Create the slots queue
	s := &slots{
		make([]queue.Item, 0, 5),
		5,
	}

	for _, chunkRange := range existingRanges {
		s.Push(&slot{
			chunkRange: chunkRange,
		})
	}

	require.True(t, s.Len() == len(existingRanges))

	// Reserve chunk ranges
	s.reserveChunkRanges(1, 50, 10)

	require.Equal(t, len(expectedRanges), s.Len())

	for index, chunkRange := range expectedRanges {
		slot := s.getSlot(index)

		assert.Equal(t, chunkRange, slot.chunkRange)
	}

	// Sanity check for double reserves
	assert.Len(t, s.reserveChunkRanges(1, 50, 10), 0)
}
