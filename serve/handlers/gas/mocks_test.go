package gas

import (
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/storage"
)

type getLatestHeight func() (uint64, error)

type txReverseIterator func(uint64, uint64, uint32, uint32) (storage.Iterator[*types.TxResult], error)

type mockStorage struct {
	getLatestHeightFn   getLatestHeight
	txReverseIteratorFn txReverseIterator
}

func (m *mockStorage) GetLatestHeight() (uint64, error) {
	if m.getLatestHeightFn != nil {
		return m.getLatestHeightFn()
	}

	return 0, nil
}

func (m *mockStorage) TxReverseIterator(
	fromTxNum,
	toTxNum uint64,
	fromIndex,
	toIndex uint32,
) (storage.Iterator[*types.TxResult], error) {
	if m.txReverseIteratorFn != nil {
		return m.txReverseIteratorFn(fromTxNum, toTxNum, fromIndex, toIndex)
	}

	return nil, nil
}
