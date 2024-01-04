package serve

import "github.com/gnolang/tx-indexer/events"

// Events is the interface for event passing
type Events interface {
	// Subscribe subscribes to specific events
	Subscribe([]events.Type) *events.Subscription

	// CancelSubscription cancels the given subscription
	CancelSubscription(events.SubscriptionID)
}
