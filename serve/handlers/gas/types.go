package gas

import (
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/storage"
)

type Storage interface {
	// GetLatestHeight returns the latest block height from the storage
	GetLatestHeight() (uint64, error)

	// TxIterator iterates over transactions, limiting the results to be between the provided block numbers
	// and transaction indexes
	TxReverseIterator(
		fromBlockNum,
		toBlockNum uint64,
		fromTxIndex,
		toTxIndex uint32,
	) (storage.Iterator[*types.TxResult], error)
}
