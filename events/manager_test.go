package events

import (
	"crypto/rand"
	"math/big"
	mathRand "math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_SubscribeCancel(t *testing.T) {
	t.Parallel()

	numSubscriptions := 10
	subscriptions := make([]*Subscription, numSubscriptions)
	defaultEvents := []Type{"dummy"}
	IDMap := make(map[SubscriptionID]bool)

	m := NewManager()
	defer m.Close()

	// Create the subscriptions
	for i := 0; i < numSubscriptions; i++ {
		subscriptions[i] = m.Subscribe(defaultEvents)

		// Check that the number is up-to-date
		assert.Equal(t, int64(i+1), m.numSubscriptions)

		// Check if a duplicate ID has been issued
		if _, ok := IDMap[subscriptions[i].ID]; ok {
			t.Fatalf("Duplicate ID entry")
		} else {
			IDMap[subscriptions[i].ID] = true
		}
	}

	// Cancel them one by one
	for indx, subscription := range subscriptions {
		m.CancelSubscription(subscription.ID)

		// Check that the number is up-to-date
		assert.Equal(t, int64(numSubscriptions-indx-1), m.numSubscriptions)

		// Check that the appropriate channel is closed
		if _, more := <-subscription.SubCh; more {
			t.Fatalf("Subscription channel not closed for index %d", indx)
		}
	}
}

func TestManager_SubscribeClose(t *testing.T) {
	t.Parallel()

	numSubscriptions := 10
	subscriptions := make([]*Subscription, numSubscriptions)
	defaultEvents := []Type{"dummy"}

	m := NewManager()

	// Create the subscriptions
	for i := 0; i < numSubscriptions; i++ {
		subscriptions[i] = m.Subscribe(defaultEvents)

		// Check that the number is up-to-date
		assert.Equal(t, int64(i+1), m.numSubscriptions)
	}

	// Close off the event manager
	m.Close()
	assert.Equal(t, int64(0), m.numSubscriptions)

	// Check if the subscription channels are closed
	for indx, subscription := range subscriptions {
		if _, more := <-subscription.SubCh; more {
			t.Fatalf("Subscription channel not closed for index %d", indx)
		}
	}
}

func TestManager_SignalEvent(t *testing.T) {
	t.Parallel()

	totalEvents := 10
	invalidEvents := 3
	validEvents := totalEvents - invalidEvents
	supportedEventTypes := []Type{"dummy1", "dummy2"}

	m := NewManager()
	defer m.Close()

	subscription := m.Subscribe(supportedEventTypes)

	eventSupported := func(eventType Type) bool {
		for _, supportedType := range supportedEventTypes {
			if supportedType == eventType {
				return true
			}
		}

		return false
	}

	mockEvents := getMockEvents(
		t,
		supportedEventTypes,
		totalEvents,
		invalidEvents,
	)

	// Send the events
	for _, mockEvent := range mockEvents {
		m.SignalEvent(mockEvent)
	}

	// Make sure all valid events get processed
	eventsProcessed := 0
	supportedEventsProcessed := 0

	completed := false
	for !completed {
		select {
		case event := <-subscription.SubCh:
			eventsProcessed++

			if eventSupported(event.GetType()) {
				supportedEventsProcessed++
			}

			if eventsProcessed == validEvents ||
				supportedEventsProcessed == validEvents {
				completed = true
			}
		case <-time.After(time.Second * 5):
			completed = true
		}
	}

	assert.Equal(t, validEvents, eventsProcessed)
	assert.Equal(t, validEvents, supportedEventsProcessed)
}

func TestManager_SignalEventOrder(t *testing.T) {
	t.Parallel()

	totalEvents := 1000
	supportedEventTypes := []Type{
		"dummy 1",
		"dummy 2",
		"dummy 3",
		"dummy 4",
		"dummy 5",
	}

	m := NewManager()
	defer m.Close()

	subscription := m.Subscribe(supportedEventTypes)

	mockEvents := getMockEvents(t, supportedEventTypes, totalEvents, 0)
	eventsProcessed := 0

	var wg sync.WaitGroup

	wg.Add(totalEvents)

	go func() {
		for {
			select {
			case event, more := <-subscription.SubCh:
				if more {
					assert.Equal(t, mockEvents[eventsProcessed].GetType(), event.GetType())

					eventsProcessed++

					wg.Done()
				}
			case <-time.After(time.Second * 5):
				for i := 0; i < totalEvents-eventsProcessed; i++ {
					wg.Done()
				}
			}
		}
	}()

	// Send the events
	for _, mockEvent := range mockEvents {
		m.SignalEvent(mockEvent)
	}

	// Make sure all valid events get processed
	wg.Wait()

	assert.Equal(t, totalEvents, eventsProcessed)
}

// getMockEvents generates mock events
// of supported types, and unsupported types,
// shuffling them in the process
func getMockEvents(
	t *testing.T,
	supportedTypes []Type,
	count int,
	numInvalid int,
) []*mockEvent {
	t.Helper()

	if count == 0 || len(supportedTypes) == 0 {
		return []*mockEvent{}
	}

	if numInvalid > count {
		numInvalid = count
	}

	allEvents := []Type{
		"random type 1",
		"random type 2",
		"random type 3",
	}

	allEvents = append(allEvents, supportedTypes...)

	tempSubscription := &eventSubscription{eventTypes: supportedTypes}

	randomEventType := func(supported bool) Type {
		for {
			randNum, err := rand.Int(rand.Reader, big.NewInt(int64(len(allEvents))))
			require.NoError(t, err)

			randType := allEvents[randNum.Int64()]
			if tempSubscription.eventSupported(randType) == supported {
				return randType
			}
		}
	}

	events := make([]*mockEvent, 0)

	// Fill in the unsupported events first
	for invalidFilled := 0; invalidFilled < numInvalid; invalidFilled++ {
		events = append(events, &mockEvent{
			eventType: randomEventType(false),
			data:      []byte("data"),
		})
	}

	// Fill in the supported events
	for validFilled := 0; validFilled < count-numInvalid; validFilled++ {
		events = append(events, &mockEvent{
			eventType: randomEventType(true),
			data:      []byte("data"),
		},
		)
	}

	// Shuffle the events
	mathRand.Shuffle(len(events), func(i, j int) {
		events[i], events[j] = events[j], events[i]
	})

	return events
}
