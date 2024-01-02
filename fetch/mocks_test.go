package fetch

import (
	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

type (
	getLatestSavedHeightDelegate func() (int64, error)
	saveBlockDelegate            func(*types.Block) error
	saveTxDelegate               func(*types.TxResult) error
)

type mockStorage struct {
	getLatestSavedHeightFn getLatestSavedHeightDelegate
	saveBlockFn            saveBlockDelegate
	saveTxFn               saveTxDelegate
}

func (m *mockStorage) GetLatestHeight() (int64, error) {
	if m.getLatestSavedHeightFn != nil {
		return m.getLatestSavedHeightFn()
	}

	return 0, nil
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
)

type mockClient struct {
	getLatestBlockNumberFn getLatestBlockNumberDelegate
	getBlockFn             getBlockDelegate
	getBlockResultsFn      getBlockResultsDelegate
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
