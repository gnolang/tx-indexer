package gas

import (
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/storage"
)

type Storage interface {
	// GetLatestHeight returns the latest block height from the storage
	GetLatestHeight() (uint64, error)

	// BlockIterator iterates over Blocks, limiting the results to be between the provided block numbers
	BlockIterator(fromBlockNum, toBlockNum uint64) (storage.Iterator[*types.Block], error)
}
