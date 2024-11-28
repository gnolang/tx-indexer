package gas

import (
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/storage"
)

type getLatestHeight func() (uint64, error)

type blockIterator func(uint64, uint64) (storage.Iterator[*types.Block], error)

type mockStorage struct {
	getLatestHeightFn getLatestHeight
	blockIteratorFn   blockIterator
}

func (m *mockStorage) GetLatestHeight() (uint64, error) {
	if m.getLatestHeightFn != nil {
		return m.getLatestHeightFn()
	}

	return 0, nil
}

func (m *mockStorage) BlockIterator(
	fromBlockNum,
	toBlockNum uint64,
) (storage.Iterator[*types.Block], error) {
	if m.blockIteratorFn != nil {
		return m.blockIteratorFn(fromBlockNum, toBlockNum)
	}

	return nil, nil
}
