package events

type eventSubscription struct {
	// eventStore is used for temporary concurrent event storage,
	// required in order to preserve the chronological order of events
	eventStore *eventQueue

	// outputCh is the update channel for the subscriber
	outputCh chan Event

	// doneCh is the channel for handling stop signals
	doneCh chan struct{}

	// notifyCh is the channel for receiving event requests
	notifyCh chan struct{}

	// eventTypes is the list of subscribed event types
	eventTypes []Type
}

// close stops the event subscription
func (es *eventSubscription) close() {
	close(es.doneCh)
}

// runLoop is the main loop that listens for notifications and handles the event / close signals
func (es *eventSubscription) runLoop() {
	defer close(es.outputCh)

	for {
		select {
		case <-es.doneCh: // Break if a close signal has been received
			return
		case <-es.notifyCh: // Listen for new events to appear
			for {
				// Grab the next event to be processed by order of sending
				event := es.eventStore.pop()
				if event == nil {
					break
				}

				select {
				case <-es.doneCh: // Break if a close signal has been received
					return
				case es.outputCh <- event: // Pass the event to the output
				}
			}
		}
	}
}

// eventSupported checks if any notification event needs to be triggered
func (es *eventSubscription) eventSupported(eventType Type) bool {
	for _, supportedType := range es.eventTypes {
		if supportedType == eventType {
			return true
		}
	}

	return false
}

// pushEvent sends the event off for processing by the subscription. [NON-BLOCKING]
func (es *eventSubscription) pushEvent(event Event) {
	if !es.eventSupported(event.GetType()) {
		return
	}

	// Append the event to the event store, so order can be preserved
	es.eventStore.push(event)

	select {
	case es.notifyCh <- struct{}{}: // Notify the worker thread
	default:
	}
}
