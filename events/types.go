package events

type (
	Type           string
	SubscriptionID int32
)

// Event is the abstraction for any event
type Event interface {
	// GetType returns the type of the event
	GetType() Type

	// GetData returns the wrapped event data
	GetData() any
}

// Subscription is the subscription
// returned to the user
type Subscription struct {
	// SubCh is the notification channel
	// on which the listener will receive notifications
	SubCh chan Event

	// ID is the unique identifier of the subscription
	ID SubscriptionID
}
