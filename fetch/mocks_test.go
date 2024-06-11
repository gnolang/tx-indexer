package fetch

import (
	"context"

	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	bft_types "github.com/gnolang/gno/tm2/pkg/bft/types"

	clientTypes "github.com/gnolang/tx-indexer/client/types"
	"github.com/gnolang/tx-indexer/events"
)

type (
	getLatestBlockNumberDelegate func() (uint64, error)
	getBlockDelegate             func(uint64) (*core_types.ResultBlock, error)
	getBlockResultsDelegate      func(uint64) (*core_types.ResultBlockResults, error)

	createBatchDelegate func() clientTypes.Batch
)

type mockClient struct {
	getLatestBlockNumberFn getLatestBlockNumberDelegate
	getBlockFn             getBlockDelegate
	getBlockResultsFn      getBlockResultsDelegate

	createBatchFn createBatchDelegate
}

func (m *mockClient) GetLatestBlockNumber() (uint64, error) {
	if m.getLatestBlockNumberFn != nil {
		return m.getLatestBlockNumberFn()
	}

	return 0, nil
}

func (m *mockClient) GetBlock(blockNum uint64) (*core_types.ResultBlock, error) {
	if m.getBlockFn != nil {
		return m.getBlockFn(blockNum)
	}

	return nil, nil
}

func (m *mockClient) GetGenesis() (*core_types.ResultGenesis, error) {
	return &core_types.ResultGenesis{Genesis: &bft_types.GenesisDoc{}}, nil
}

func (m *mockClient) GetBlockResults(blockNum uint64) (*core_types.ResultBlockResults, error) {
	if m.getBlockResultsFn != nil {
		return m.getBlockResultsFn(blockNum)
	}

	return nil, nil
}

func (m *mockClient) CreateBatch() clientTypes.Batch {
	if m.createBatchFn != nil {
		return m.createBatchFn()
	}

	return nil
}

type (
	addBlockRequestDelegate        func(uint64) error
	addBlockResultsRequestDelegate func(uint64) error
	executeDelegate                func(context.Context) ([]any, error)
	countDelegate                  func() int
)

type mockBatch struct {
	addBlockRequestFn        addBlockRequestDelegate
	addBlockResultsRequestFn addBlockResultsRequestDelegate
	executeFn                executeDelegate
	countFn                  countDelegate
}

func (m *mockBatch) AddBlockRequest(num uint64) error {
	if m.addBlockRequestFn != nil {
		return m.addBlockRequestFn(num)
	}

	return nil
}

func (m *mockBatch) AddBlockResultsRequest(num uint64) error {
	if m.addBlockResultsRequestFn != nil {
		return m.addBlockResultsRequestFn(num)
	}

	return nil
}

func (m *mockBatch) Execute(ctx context.Context) ([]any, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx)
	}

	return nil, nil
}

func (m *mockBatch) Count() int {
	if m.countFn != nil {
		return m.countFn()
	}

	return 0
}

type (
	signalEventDelegate func(events.Event)
)

type mockEvents struct {
	signalEventFn signalEventDelegate
}

func (m *mockEvents) SignalEvent(event events.Event) {
	if m.signalEventFn != nil {
		m.signalEventFn(event)
	}
}
