package block

import (
	"context"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

type Storage interface {
	// GetBlock returns specified block from permanent storage
	GetBlock(ctx context.Context, blockNum uint64) (*types.Block, error)
}
