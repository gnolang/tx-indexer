package fetch

import (
	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	clientTypes "github.com/gnolang/tx-indexer/client/types"
)

type (
	getLatestHeightDelegate  func() (int64, error)
	saveLatestHeightDelegate func(int64) error
	saveBlockDelegate        func(*types.Block) error
	saveTxDelegate           func(*types.TxResult) error
)

type mockStorage struct {
	getLatestSavedHeightFn getLatestHeightDelegate
	saveLatestHeightFn     saveLatestHeightDelegate
	saveBlockFn            saveBlockDelegate
	saveTxFn               saveTxDelegate
}

func (m *mockStorage) GetLatestHeight() (int64, error) {
	if m.getLatestSavedHeightFn != nil {
		return m.getLatestSavedHeightFn()
	}

	return 0, nil
}

func (m *mockStorage) SaveLatestHeight(blockNum int64) error {
	if m.saveLatestHeightFn != nil {
		return m.saveLatestHeightFn(blockNum)
	}

	return nil
}

func (m *mockStorage) SaveTx(tx *types.TxResult) error {
	if m.saveTxFn != nil {
		return m.saveTxFn(tx)
	}

	return nil
}

func (m *mockStorage) SaveBlock(block *types.Block) error {
	if m.saveBlockFn != nil {
		return m.saveBlockFn(block)
	}

	return nil
}

type (
	getLatestBlockNumberDelegate func() (int64, error)
	getBlockDelegate             func(int64) (*core_types.ResultBlock, error)
	getBlockResultsDelegate      func(int64) (*core_types.ResultBlockResults, error)

	createBatchDelegate func() clientTypes.Batch
)

type mockClient struct {
	getLatestBlockNumberFn getLatestBlockNumberDelegate
	getBlockFn             getBlockDelegate
	getBlockResultsFn      getBlockResultsDelegate

	createBatchFn createBatchDelegate
}

func (m *mockClient) GetLatestBlockNumber() (int64, error) {
	if m.getLatestBlockNumberFn != nil {
		return m.getLatestBlockNumberFn()
	}

	return 0, nil
}

func (m *mockClient) GetBlock(blockNum int64) (*core_types.ResultBlock, error) {
	if m.getBlockFn != nil {
		return m.getBlockFn(blockNum)
	}

	return nil, nil
}

func (m *mockClient) GetBlockResults(blockNum int64) (*core_types.ResultBlockResults, error) {
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
