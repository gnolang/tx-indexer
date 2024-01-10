package types

import (
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/events"
)

// NewBlockEvent is the event for when new blocks appear
var NewBlockEvent events.Type = "newHeads"

type NewBlock struct {
	Block   *types.Block
	Results []*types.TxResult
}

func (n *NewBlock) GetType() events.Type {
	return NewBlockEvent
}

func (n *NewBlock) GetData() any {
	return n
}
