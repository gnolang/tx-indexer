package subscription

import (
	"fmt"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/serve/conns"
	"github.com/gnolang/tx-indexer/serve/methods"
	"github.com/gnolang/tx-indexer/serve/spec"
)

const (
	NewGasPriceEvent = "newGasPrice"
)

// GasPriceSubscription is the new-transactions type
// subscription
type GasPriceSubscription struct {
	*baseSubscription
}

func NewGasPriceSubscription(conn conns.WSConnection) *GasPriceSubscription {
	return &GasPriceSubscription{
		baseSubscription: newBaseSubscription(conn),
	}
}

func (b *GasPriceSubscription) GetType() events.Type {
	return NewGasPriceEvent
}

func (b *GasPriceSubscription) WriteResponse(id string, data any) error {
	block, ok := data.(*types.Block)
	if !ok {
		return fmt.Errorf("unable to cast txResult, %s", data)
	}

	gasPrices, err := methods.GetGasPricesByBlock(block)
	if err != nil {
		return err
	}

	return b.conn.WriteData(spec.NewJSONSubscribeResponse(id, gasPrices))
}
