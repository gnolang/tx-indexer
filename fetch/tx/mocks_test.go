package tx

import (
	"context"

	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

type (
	getLatestTxDelegate func(context.Context) (*types.TxResult, error)
	saveTxDelegate      func(context.Context, *types.TxResult) error
)

type mockStorage struct {
	getLatestTxFn getLatestTxDelegate
	saveTxFn      saveTxDelegate
}

func (m *mockStorage) GetLatestTx(ctx context.Context) (*types.TxResult, error) {
	if m.getLatestTxFn != nil {
		return m.getLatestTxFn(ctx)
	}

	return nil, nil
}

func (m *mockStorage) SaveTx(ctx context.Context, tx *types.TxResult) error {
	if m.saveTxFn != nil {
		return m.saveTxFn(ctx, tx)
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
