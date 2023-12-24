package events

import "sync"

type eventQueue struct {
	events []Event

	sync.Mutex
}

func (es *eventQueue) push(event Event) {
	es.Lock()
	defer es.Unlock()

	es.events = append(es.events, event)
}

func (es *eventQueue) pop() Event {
	es.Lock()
	defer es.Unlock()

	if len(es.events) == 0 {
		return nil
	}

	event := es.events[0]
	es.events = es.events[1:]

	return event
}
