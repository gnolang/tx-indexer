package block

import "github.com/gnolang/gno/tm2/pkg/bft/types"

type getBlockDelegate func(int64) (*types.Block, error)

type mockStorage struct {
	getBlockFn getBlockDelegate
}

func (m *mockStorage) GetBlock(num int64) (*types.Block, error) {
	if m.getBlockFn != nil {
		return m.getBlockFn(num)
	}

	return nil, nil
}
