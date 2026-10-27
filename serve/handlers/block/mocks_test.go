package block

import (
	"context"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

type getBlockDelegate func(uint64) (*types.Block, error)

type mockStorage struct {
	getBlockFn getBlockDelegate
}

func (m *mockStorage) GetBlock(_ context.Context, num uint64) (*types.Block, error) {
	if m.getBlockFn != nil {
		return m.getBlockFn(num)
	}

	return nil, nil
}
