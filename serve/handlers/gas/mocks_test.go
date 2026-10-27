package gas

import (
	"context"

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

type mockIterator struct {
	nextFn  func() bool
	valueFn func() (*types.Block, error)
	errorFn func() error
}

func (m *mockIterator) Next(context.Context) bool {
	if m.nextFn != nil {
		return m.nextFn()
	}

	return false
}

func (m *mockIterator) Value() (*types.Block, error) {
	if m.valueFn != nil {
		return m.valueFn()
	}

	return nil, nil
}

func (m *mockIterator) Error() error {
	if m.errorFn != nil {
		return m.errorFn()
	}

	return nil
}

func (m *mockIterator) Close() error {
	return nil
}
