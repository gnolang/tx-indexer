package fetch

import (
	"context"

	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"

	clientTypes "github.com/gnolang/tx-indexer/client/types"
	"github.com/gnolang/tx-indexer/events"
)

// Client defines the interface for the node (client) communication
type Client interface {
	// GetLatestBlockNumber returns the latest block height from the chain
	GetLatestBlockNumber(context.Context) (uint64, error)

	// GetBlock returns specified block
	GetBlock(context.Context, uint64) (*core_types.ResultBlock, error)

	// GetGenesis returns the genesis block
	GetGenesis(context.Context) (*core_types.ResultGenesis, error)

	// GetBlockResults returns the results of executing the transactions
	// for the specified block
	GetBlockResults(context.Context, uint64) (*core_types.ResultBlockResults, error)

	// CreateBatch creates a new client batch
	CreateBatch() clientTypes.Batch
}

// Events is the events API
type Events interface {
	// SignalEvent signals a new event to the event manager
	SignalEvent(events.Event)
}
