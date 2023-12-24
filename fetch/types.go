package fetch

import (
	"context"

	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

// Storage defines the transaction storage interface
type Storage interface {
	// GetLatestSavedHeight returns the latest block height from the storage
	GetLatestSavedHeight(ctx context.Context) (int64, error)

	SaveBlock(ctx context.Context, block *types.Block) error

	// SaveTx saves the transaction to the permanent storage
	SaveTx(ctx context.Context, tx *types.TxResult) error
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
}
