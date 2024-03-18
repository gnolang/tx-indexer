package block

import (
	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

type Storage interface {
	// GetBlock returns specified block from permanent storage
	GetBlock(uint64) (*types.Block, error)
}
