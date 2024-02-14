package mock

import (
	"github.com/gnolang/tx-indexer/events"
)

type (
	writeDataFnDelegate func(any) error
)

type Conn struct {
	WriteDataFn writeDataFnDelegate
}

func (m *Conn) WriteData(data any) error {
	if m.WriteDataFn != nil {
		return m.WriteDataFn(data)
	}

	return nil
}

type (
	subscribeDelegate          func([]events.Type) *events.Subscription
	cancelSubscriptionDelegate func(events.SubscriptionID)
)

type Events struct {
	SubscribeFn          subscribeDelegate
	CancelSubscriptionFn cancelSubscriptionDelegate
}

func (m *Events) Subscribe(eventTypes []events.Type) *events.Subscription {
	if m.SubscribeFn != nil {
		return m.SubscribeFn(eventTypes)
	}

	return nil
}

func (m *Events) CancelSubscription(id events.SubscriptionID) {
	if m.CancelSubscriptionFn != nil {
		m.CancelSubscriptionFn(id)
	}
}
