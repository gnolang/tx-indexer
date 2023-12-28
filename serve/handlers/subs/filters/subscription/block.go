package subscription

import (
	"github.com/gnolang/tx-indexer/serve/conns"
	"github.com/gnolang/tx-indexer/serve/spec"
	"github.com/gnolang/tx-indexer/types"
)

const (
	NewHeadsEvent = "newHeads"
)

// BlockSubscription is the new-heads type
// subscription
type BlockSubscription struct {
	*baseSubscription
}

func NewBlockSubscription(conn conns.WSConnection) *BlockSubscription {
	return &BlockSubscription{
		baseSubscription: newBaseSubscription(conn),
	}
}

func (b *BlockSubscription) WriteResponse(id string, block types.Block) error {
	return b.conn.WriteData(spec.NewJSONSubscribeResponse(id, block))
}
