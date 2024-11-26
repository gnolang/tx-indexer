package gas

import (
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/storage"
)

type Storage interface {
	// GetTx returns specified tx from permanent storage
	GetLatestHeight() (uint64, error)

	// GetTxByHash fetches the tx using the transaction hash
	TxIterator(
		fromBlockNum,
		toBlockNum uint64,
		fromTxIndex,
		toTxIndex uint32,
	) (storage.Iterator[*types.TxResult], error)
}
