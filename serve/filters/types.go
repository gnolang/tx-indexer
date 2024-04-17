package filters

import (
	"errors"
	"time"

	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/serve/filters/filter"
)

var ErrFilterNotFound = errors.New("filter not found")

// Events is the interface for event passing
type Events interface {
	// Subscribe subscribes to specific events
	Subscribe([]events.Type) *events.Subscription

	// CancelSubscription cancels the given subscription
	CancelSubscription(events.SubscriptionID)
}

// Filter interface is used for different filter types
type Filter interface {
	// GetType returns the filter type
	GetType() filter.Type

	// GetLastUsed returns the time the filter was last used
	GetLastUsed() time.Time

	// UpdateLastUsed updates the last used time
	UpdateLastUsed()

	// GetChanges returns any filter changes (specific to the filter type)
	GetChanges() any

	// UpdateWith updates the specific filter type with a event's data
	UpdateWith(data any)
}
