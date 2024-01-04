package filters

import (
	"errors"
	"time"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
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

// Storage represents the permanent storage abstraction
// required by the filter manager
type Storage interface {
	// GetBlock fetches the block by its number
	GetBlock(int64) (*types.Block, error)

	// GetTx fetches the tx using its hash
	GetTx([]byte) (*types.TxResult, error)
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

	// UpdateWithBlock updates the specific filter type with a new block
	UpdateWithBlock(block *types.Block)
}
