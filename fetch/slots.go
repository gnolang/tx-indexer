package fetch

import (
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	queue "github.com/madz-lab/insertion-queue"
)

type chunk struct {
	blocks  []*types.Block
	results []*types.TxResult
}

// slot is a single chunk slot
type slot struct {
	chunk      *chunk     // retrieved data chunk
	chunkRange chunkRange // retrieved data chunk range
}

func (s *slot) Less(i queue.Item) bool {
	other, ok := i.(*slot)
	if !ok {
		return false
	}

	return s.chunkRange.less(other.chunkRange)
}

// chunkRange is the data sequence range for the data
type chunkRange struct {
	from int64 // sequence from (inclusive)
	to   int64 // sequence to (inclusive)
}

// less returns a flag indicating if the current chunk range is less than the other
func (c chunkRange) less(other chunkRange) bool {
	return c.from < other.from
}

// slots is the fixed priority-queue slot representation
type slots struct {
	queue.Queue

	maxSlots int
}

// getSlot fetches the slot at the specific index
func (s *slots) getSlot(index int) *slot {
	if s.Len()-1 < index {
		return nil
	}

	return s.Index(index).(*slot)
}

// setChunk sets the chunk for the specified index
func (s *slots) setChunk(index int, chunk *chunk) {
	item := s.getSlot(index)
	item.chunk = chunk

	s.Queue[index] = item
}

// reserveChunkRanges reserves empty chunk ranges, and returns them, if any
func (s *slots) reserveChunkRanges(start, end, maxChunkSize int64) []chunkRange {
	freeSlots := s.maxSlots - s.Len()

	gaps := s.findGaps(start, end, maxChunkSize)
	maxRanges := min(len(gaps), freeSlots)
	chunkRanges := make([]chunkRange, maxRanges)

	for index, gap := range gaps[:maxRanges] {
		chunkRanges[index] = gap

		s.Push(&slot{
			chunk:      nil,
			chunkRange: gap,
		})
	}

	return chunkRanges
}

// findGaps finds the chunk gaps in the specified range
func (s *slots) findGaps(start, end, maxSize int64) []chunkRange {
	var (
		chunkRanges []chunkRange // contains all gaps
		dividedGaps []chunkRange // contains at most maxSize gaps
	)

	if s.Len() == 0 {
		chunkRanges = append(chunkRanges, chunkRange{
			from: start,
			to:   end,
		})
	} else {
		prevTo := start - 1

		for _, partRaw := range s.Queue {
			part := partRaw.(*slot)

			if part.chunkRange.from > prevTo+1 {
				chunkRanges = append(chunkRanges, chunkRange{
					from: prevTo + 1,
					to:   part.chunkRange.from - 1,
				})
			}

			prevTo = part.chunkRange.to
		}

		if prevTo < end {
			chunkRanges = append(chunkRanges, chunkRange{
				from: prevTo + 1,
				to:   end,
			})
		}
	}

	for _, gap := range chunkRanges {
		if gap.to-gap.from+1 <= maxSize {
			dividedGaps = append(dividedGaps, gap)

			continue
		}

		for i := gap.from; i <= gap.to; i += maxSize {
			newGap := chunkRange{
				from: i,
				to:   i + maxSize - 1,
			}

			if newGap.to > gap.to {
				newGap.to = gap.to
			}

			dividedGaps = append(dividedGaps, newGap)
		}
	}

	return dividedGaps
}
