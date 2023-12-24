package events

import (
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

// Manager is the subscription manager
type Manager struct {
	subscriptions    map[SubscriptionID]*eventSubscription
	numSubscriptions int64

	subscriptionsLock sync.RWMutex
}

// NewManager creates a new instance
// of the subscription manager
func NewManager() *Manager {
	return &Manager{
		subscriptions:    make(map[SubscriptionID]*eventSubscription),
		numSubscriptions: 0,
	}
}

// Subscribe registers a new listener for events
func (em *Manager) Subscribe(eventTypes []Type) *Subscription {
	em.subscriptionsLock.Lock()
	defer em.subscriptionsLock.Unlock()

	id := uuid.New().ID()
	subscription := &eventSubscription{
		eventTypes: eventTypes,
		outputCh:   make(chan Event, 1),
		doneCh:     make(chan struct{}),
		notifyCh:   make(chan struct{}, 1),
		eventStore: &eventQueue{
			events: make([]Event, 0),
		},
	}

	em.subscriptions[SubscriptionID(id)] = subscription

	go subscription.runLoop()

	atomic.AddInt64(&em.numSubscriptions, 1)

	return &Subscription{
		ID:    SubscriptionID(id),
		SubCh: subscription.outputCh,
	}
}

// CancelSubscription stops a subscription for events
func (em *Manager) CancelSubscription(id SubscriptionID) {
	em.subscriptionsLock.Lock()
	defer em.subscriptionsLock.Unlock()

	if subscription, ok := em.subscriptions[id]; ok {
		subscription.close()
		delete(em.subscriptions, id)

		atomic.AddInt64(&em.numSubscriptions, -1)
	}
}

// Close stops the event manager, effectively cancelling all subscriptions
func (em *Manager) Close() {
	em.subscriptionsLock.Lock()
	defer em.subscriptionsLock.Unlock()

	for _, subscription := range em.subscriptions {
		subscription.close()
	}

	atomic.StoreInt64(&em.numSubscriptions, 0)
}

// SignalEvent is a helper method for alerting listeners of a new message event
func (em *Manager) SignalEvent(event Event) {
	if atomic.LoadInt64(&em.numSubscriptions) == 0 {
		// No reason to lock the subscriptions map
		// if no subscriptions exist
		return
	}

	em.subscriptionsLock.RLock()
	defer em.subscriptionsLock.RUnlock()

	for _, subscription := range em.subscriptions {
		subscription.pushEvent(event)
	}
}
