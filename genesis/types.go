package genesis

import (
	"context"

	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"

	"github.com/gnolang/tx-indexer/storage"
)

// Client is the chain access the bootstrap needs
type Client interface {
	// GetGenesis returns the chain genesis document
	GetGenesis(context.Context) (*core_types.ResultGenesis, error)

	// GetStatus returns the chain status, for the chain ID the stored genesis
	// is validated against
	GetStatus(context.Context) (*core_types.ResultStatus, error)

	// GetBlockResults returns the results of executing the transactions
	// for the specified block
	GetBlockResults(context.Context, uint64) (*core_types.ResultBlockResults, error)
}

// Storage is the storage access the bootstrap needs. Narrow on purpose: the
// chain ID says whether there is any work to do, and the latest height says
// whether the genesis block is already indexed
type Storage interface {
	// GetLatestHeight returns the latest block height from the storage
	GetLatestHeight() (uint64, error)

	// GetGenesisChainID returns the chain ID of the bootstrapped genesis
	GetGenesisChainID() (string, error)

	// WriteBatch provides a batch intended to do a write action that
	// can be cancelled or committed all at the same time
	WriteBatch() storage.Batch
}
