package subscription

import (
	"fmt"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/serve/conns"
	"github.com/gnolang/tx-indexer/serve/encode"
	"github.com/gnolang/tx-indexer/serve/spec"
)

const (
	NewTransactionsEvent = "newTransactions"
)

// TransactionSubscription is the new-heads type
// subscription
type TransactionSubscription struct {
	*baseSubscription
}

func NewTransactionSubscription(conn conns.WSConnection) *TransactionSubscription {
	return &TransactionSubscription{
		baseSubscription: newBaseSubscription(conn),
	}
}

func (b *TransactionSubscription) WriteResponse(id string, data any) error {
	tx, ok := data.(*types.TxResult)
	if !ok {
		return fmt.Errorf("unable to cast txResult, %s", data)
	}

	encodedTx, err := encode.PrepareValue(tx)
	if err != nil {
		return err
	}

	return b.conn.WriteData(spec.NewJSONSubscribeResponse(id, encodedTx))
}
