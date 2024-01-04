package fetch

import (
	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	clientTypes "github.com/gnolang/tx-indexer/client/types"
	"github.com/gnolang/tx-indexer/events"
)

// Storage defines the transaction storage interface
type Storage interface {
	// GetLatestHeight returns the latest block height from the storage
	GetLatestHeight() (int64, error)

	// SaveLatestHeight saves the latest block height to the storage
	SaveLatestHeight(int64) error

	// SaveBlock saves the block to the permanent storage
	SaveBlock(block *types.Block) error

	// SaveTx saves the transaction to the permanent storage
	SaveTx(tx *types.TxResult) error
}

// Client defines the interface for the node (client) communication
type Client interface {
	// GetLatestBlockNumber returns the latest block height from the chain
	GetLatestBlockNumber() (int64, error)

	// GetBlock returns specified block
	GetBlock(int64) (*core_types.ResultBlock, error)

	// GetBlockResults returns the results of executing the transactions
	// for the specified block
	GetBlockResults(int64) (*core_types.ResultBlockResults, error)

	// CreateBatch creates a new client batch
	CreateBatch() clientTypes.Batch
}

// Events is the events API
type Events interface {
	// SignalEvent signals a new event to the event manager
	SignalEvent(events.Event)
}
