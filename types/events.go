package types

import (
	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/events"
)

// NewBlockEvent is the event for when new blocks appear
var NewBlockEvent events.Type = "newHeads"

type NewBlock struct {
	Block   *types.Block
	Results *core_types.ResultBlockResults
}

func (n *NewBlock) GetType() events.Type {
	return NewBlockEvent
}

func (n *NewBlock) GetData() any {
	return n
}
