package filters

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// filterMap keeps atomically keeps track
// of active filters
type filterMap struct {
	items map[string]Filter

	sync.Mutex
}

// newFilterMap creates a new filter map
func newFilterMap() *filterMap {
	return &filterMap{
		items: make(map[string]Filter),
	}
}

// newFilter adds a new filter to the filter map,
// and returns the corresponding ID
func (f *filterMap) newFilter(filter Filter) string {
	f.Lock()
	defer f.Unlock()

	// Crete new id
	id := uuid.New().String()

	// Add filter to the map
	f.items[id] = filter

	return id
}

// uninstallFilter removes the filter with the specified ID.
// Returns a flag indicating if the filter was removed
func (f *filterMap) uninstallFilter(id string) bool {
	f.Lock()
	defer f.Unlock()

	_, exists := f.items[id]

	delete(f.items, id)

	return exists
}

// getFilter fetches the filter using the filter ID
func (f *filterMap) getFilter(id string) Filter {
	f.Lock()
	defer f.Unlock()

	filterItem, ok := f.items[id]
	if !ok {
		return nil
	}

	// Update when was the filter last used (accessed)
	filterItem.UpdateLastUsed()
	f.items[id] = filterItem

	return filterItem
}

// cleanup removes any filters that have been last used
// before the specified cutoff
func (f *filterMap) cleanup(cutoff time.Time) {
	f.Lock()
	defer f.Unlock()

	// Iterate over filters and remove which haven't been used in a while
	for key, filterItem := range f.items {
		if filterItem.GetLastUsed().Before(cutoff) {
			delete(f.items, key)
		}
	}
}

// rangeItems ranges over the filter map with the specified callback
func (f *filterMap) rangeItems(cb func(filter Filter)) {
	f.Lock()
	defer f.Unlock()

	for _, item := range f.items {
		cb(item)
	}
}
