package gas

import (
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/storage"
)

type getLatestHeight func() (uint64, error)

type txIterator func(uint64, uint64, uint32, uint32) (storage.Iterator[*types.TxResult], error)

type mockStorage struct {
	getLatestHeightFn getLatestHeight
	txIteratorFn      txIterator
}

func (m *mockStorage) GetLatestHeight() (uint64, error) {
	if m.getLatestHeightFn != nil {
		return m.getLatestHeightFn()
	}

	return 0, nil
}

func (m *mockStorage) TxIterator(
	fromBlockNum,
	toBlockNum uint64,
	fromTxIndex,
	toTxIndex uint32,
) (storage.Iterator[*types.TxResult], error) {
	if m.txIteratorFn != nil {
		return m.txIteratorFn(fromBlockNum, toBlockNum, fromTxIndex, toTxIndex)
	}

	return nil, nil
}
