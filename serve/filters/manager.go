package filters

import (
	"context"
	"fmt"
	"time"

	"github.com/gnolang/gno/tm2/pkg/bft/types"

	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/serve/conns"
	"github.com/gnolang/tx-indexer/serve/filters/filter"
	filterSubscription "github.com/gnolang/tx-indexer/serve/filters/subscription"
	"github.com/gnolang/tx-indexer/storage"
	commonTypes "github.com/gnolang/tx-indexer/types"
)

// Manager manages all running filters
type Manager struct {
	ctx             context.Context
	storage         storage.Storage // temporarily unused
	events          Events
	filters         *filterMap
	subscriptions   *subscriptionMap
	cleanupInterval time.Duration
}

// NewFilterManager creates new filter manager object
func NewFilterManager(
	ctx context.Context,
	storage storage.Storage,
	events Events,
	opts ...Option,
) *Manager {
	filterManager := &Manager{
		ctx:             ctx,
		storage:         storage,
		events:          events,
		filters:         newFilterMap(),
		subscriptions:   newSubMap(),
		cleanupInterval: 5 * time.Minute,
	}

	// Apply the options
	for _, opt := range opts {
		opt(filterManager)
	}

	// Subscribe to new block events
	go filterManager.subscribeToNewBlockEvent()

	// Start cleanup routine
	go filterManager.cleanupRoutine()

	return filterManager
}

// NewBlockFilter creates a new block filter, and returns the corresponding ID
func (f *Manager) NewBlockFilter() string {
	blockFilter := filter.NewBlockFilter()

	return f.filters.newFilter(blockFilter)
}

// UninstallFilter removes a filter from the filter map using its ID.
// Returns a flag indicating if the filter was removed
func (f *Manager) UninstallFilter(id string) bool {
	return f.filters.uninstallFilter(id)
}

// NewBlockSubscription creates a new block (new heads) subscription (over WS)
func (f *Manager) NewBlockSubscription(conn conns.WSConnection) string {
	return f.newSubscription(filterSubscription.NewBlockSubscription(conn))
}

// newSubscription adds new subscription to the subscription map
func (f *Manager) newSubscription(subscription subscription) string {
	return f.subscriptions.addSubscription(subscription)
}

// UninstallSubscription removes a subscription from the subscription map.
// Returns a flag indicating if the subscription has been removed
func (f *Manager) UninstallSubscription(id string) bool {
	return f.subscriptions.deleteSubscription(id)
}

// subscribeToNewBlockEvent subscribes to new block events
func (f *Manager) subscribeToNewBlockEvent() {
	blockSub := f.events.Subscribe([]events.Type{commonTypes.NewBlockEvent})
	defer f.events.CancelSubscription(blockSub.ID)

	for {
		select {
		case <-f.ctx.Done():
			return
		case blockRaw, more := <-blockSub.SubCh:
			if !more {
				return
			}

			// The following code segments
			// cannot be executed in parallel (go routines)
			// because data sequencing should be persisted
			// (info about block X comes before info on block X + 1)
			newBlock := blockRaw.(*commonTypes.NewBlock)

			// Apply block to filters
			f.updateFiltersWithBlock(newBlock.Block)

			// send events to all matching subscriptions
			f.subscriptions.sendBlockEvent(newBlock.Block)
		}
	}
}

// updateFiltersWithBlock updates all filters with the incoming block
func (f *Manager) updateFiltersWithBlock(block *types.Block) {
	f.filters.rangeItems(func(filter Filter) {
		filter.UpdateWithBlock(block)
	})
}

// cleanupRoutine periodically cleans up filter manager
func (f *Manager) cleanupRoutine() {
	ticker := time.NewTicker(f.cleanupInterval)

	for {
		select {
		case <-f.ctx.Done():
			return
		case <-ticker.C:
			// Cutoff time for unused filters
			cutoff := time.Now().Add(-f.cleanupInterval)

			f.filters.cleanup(cutoff)
		}
	}
}

// GetFilter returns filter with specified id
func (f *Manager) GetFilter(id string) (Filter, error) {
	filterItem := f.filters.getFilter(id)
	if filterItem == nil {
		return nil, fmt.Errorf("%w, id: %s", ErrFilterNotFound, id)
	}

	return filterItem, nil
}
