package genesis

import (
	"context"

	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
)

type mockClient struct {
	getGenesisFn      func() (*core_types.ResultGenesis, error)
	getStatusFn       func() (*core_types.ResultStatus, error)
	getBlockResultsFn func(uint64) (*core_types.ResultBlockResults, error)
}

func (m *mockClient) GetGenesis(_ context.Context) (*core_types.ResultGenesis, error) {
	if m.getGenesisFn != nil {
		return m.getGenesisFn()
	}

	return nil, nil
}

func (m *mockClient) GetStatus(_ context.Context) (*core_types.ResultStatus, error) {
	if m.getStatusFn != nil {
		return m.getStatusFn()
	}

	return &core_types.ResultStatus{}, nil
}

func (m *mockClient) GetBlockResults(_ context.Context, blockNum uint64) (*core_types.ResultBlockResults, error) {
	if m.getBlockResultsFn != nil {
		return m.getBlockResultsFn(blockNum)
	}

	return nil, nil
}
