package subscription

import (
	"fmt"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/serve/conns"
	"github.com/gnolang/tx-indexer/serve/encode"
	"github.com/gnolang/tx-indexer/serve/spec"
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

func (b *BlockSubscription) WriteResponse(id string, data any) error {
	block, ok := data.(*types.Block)
	if !ok {
		return fmt.Errorf("unable to cast block, %s", data)
	}

	encodedBlock, err := encode.PrepareValue(block.Header)
	if err != nil {
		return err
	}

	return b.conn.WriteData(spec.NewJSONSubscribeResponse(id, encodedBlock))
}
