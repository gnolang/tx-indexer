package filters

import (
	"sync"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/google/uuid"
)

type subscription interface {
	WriteResponse(id string, data any) error
}

// subscriptionMap keeps track of ongoing data subscriptions
type subscriptionMap struct {
	subscriptions map[string]subscription

	sync.Mutex
}

// newSubMap creates a new subscription map
func newSubMap() *subscriptionMap {
	return &subscriptionMap{
		subscriptions: make(map[string]subscription),
	}
}

// addSubscription adds a new subscription to the subscription map, returning its ID
func (sm *subscriptionMap) addSubscription(sub subscription) string {
	sm.Lock()
	defer sm.Unlock()

	// Crete new id
	id := uuid.New().String()

	// Add subscription to the map
	sm.subscriptions[id] = sub

	return id
}

// sendBlockEvent alerts all active subscriptions of a block event.
// In case there was an error during writing, the subscription is removed
func (sm *subscriptionMap) sendBlockEvent(block *types.Block) {
	sm.Lock()
	defer sm.Unlock()

	var (
		invalidSends = make([]string, 0, len(sm.subscriptions))

		invalidSendsMux sync.Mutex
		wg              sync.WaitGroup
	)

	markInvalid := func(id string) {
		invalidSendsMux.Lock()
		defer invalidSendsMux.Unlock()

		invalidSends = append(invalidSends, id)
	}

	for id, sub := range sm.subscriptions {
		sub := sub

		wg.Add(1)

		go func(id string) {
			defer wg.Done()

			if err := sub.WriteResponse(id, block); err != nil {
				markInvalid(id)
			}
		}(id)
	}

	wg.Wait()

	// Prune out the invalid subscriptions
	for _, invalidID := range invalidSends {
		delete(sm.subscriptions, invalidID)
	}
}

// sendBlockEvent alerts all active subscriptions of a block event.
// In case there was an error during writing, the subscription is removed
func (sm *subscriptionMap) sendTransactionEvent(txResult *types.TxResult) {
	sm.Lock()
	defer sm.Unlock()

	var (
		invalidSends = make([]string, 0, len(sm.subscriptions))

		invalidSendsMux sync.Mutex
		wg              sync.WaitGroup
	)

	markInvalid := func(id string) {
		invalidSendsMux.Lock()
		defer invalidSendsMux.Unlock()

		invalidSends = append(invalidSends, id)
	}

	for id, sub := range sm.subscriptions {
		sub := sub

		wg.Add(1)

		go func(id string) {
			defer wg.Done()

			if err := sub.WriteResponse(id, txResult); err != nil {
				markInvalid(id)
			}
		}(id)
	}

	wg.Wait()

	// Prune out the invalid subscriptions
	for _, invalidID := range invalidSends {
		delete(sm.subscriptions, invalidID)
	}
}

// deleteSubscription removes a subscription using the ID.
// Returns a flag indicating if the subscription was indeed present and removedd
func (sm *subscriptionMap) deleteSubscription(id string) bool {
	sm.Lock()
	defer sm.Unlock()

	// If the subscription exists, remove it
	_, exists := sm.subscriptions[id]
	if exists {
		delete(sm.subscriptions, id)

		return true
	}

	return false
}
