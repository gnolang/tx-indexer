package types

import (
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/events"
)

// NewBlockEvent is the event for when new blocks appear
var (
	NewBlockEvent        events.Type = "newHeads"
	NewTransactionsEvent events.Type = "newTransactions"
)

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

type NewTransaction struct {
	TxResult *types.TxResult
}

func (n *NewTransaction) GetType() events.Type {
	return NewTransactionsEvent
}

func (n *NewTransaction) GetData() any {
	return n
}
