//go:build testmocks

package mocks

import (
	tm2Types "github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/events"
)

type (
	hashDelegate   func() []byte
	headerDelegate func() *tm2Types.Header
)

type MockBlock struct {
	HashFn   hashDelegate
	HeaderFn headerDelegate
}

func (m *MockBlock) Hash() []byte {
	if m.HashFn != nil {
		return m.HashFn()
	}

	return nil
}

func (m *MockBlock) Header() *tm2Types.Header {
	if m.HeaderFn != nil {
		return m.HeaderFn()
	}

	return nil
}

type (
	writeDataFnDelegate func(any) error
)

type MockConn struct {
	WriteDataFn writeDataFnDelegate
}

func (m *MockConn) WriteData(data any) error {
	if m.WriteDataFn != nil {
		return m.WriteDataFn(data)
	}

	return nil
}

type (
	getBlockDelegate func(int64) (*tm2Types.Block, error)
	getTxDelegate    func([]byte) (*tm2Types.TxResult, error)
)

type MockStorage struct {
	GetBlockFn getBlockDelegate
	GetTxFn    getTxDelegate
}

func (m *MockStorage) GetBlock(blockNum int64) (*tm2Types.Block, error) {
	if m.GetBlockFn != nil {
		return m.GetBlockFn(blockNum)
	}

	return nil, nil
}

func (m *MockStorage) GetTx(hash []byte) (*tm2Types.TxResult, error) {
	if m.GetTxFn != nil {
		return m.GetTxFn(hash)
	}

	return nil, nil
}

type (
	subscribeDelegate          func([]events.Type) *events.Subscription
	cancelSubscriptionDelegate func(events.SubscriptionID)
)

type MockEvents struct {
	SubscribeFn          subscribeDelegate
	CancelSubscriptionFn cancelSubscriptionDelegate
}

func (m *MockEvents) Subscribe(eventTypes []events.Type) *events.Subscription {
	if m.SubscribeFn != nil {
		return m.SubscribeFn(eventTypes)
	}

	return nil
}

func (m *MockEvents) CancelSubscription(id events.SubscriptionID) {
	if m.CancelSubscriptionFn != nil {
		m.CancelSubscriptionFn(id)
	}
}
