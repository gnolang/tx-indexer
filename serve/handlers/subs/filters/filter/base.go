package filter

import (
	"sync"
	"time"
)

// baseFilter defines the common properties
// for all filter types
type baseFilter struct {
	lastUsed   time.Time
	filterType Type

	sync.RWMutex
}

func newBaseFilter(filterType Type) *baseFilter {
	return &baseFilter{
		filterType: filterType,
		lastUsed:   time.Now(),
	}
}

func (b *baseFilter) GetType() Type {
	return b.filterType
}

func (b *baseFilter) GetLastUsed() time.Time {
	b.RLock()
	defer b.RUnlock()

	return b.lastUsed
}

func (b *baseFilter) UpdateLastUsed() {
	b.Lock()
	defer b.Unlock()

	b.lastUsed = time.Now()
}

func (b *baseFilter) GetChanges() any {
	return nil
}
