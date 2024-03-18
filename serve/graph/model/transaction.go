package model

import (
	"fmt"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

type Transaction struct {
	t *types.TxResult
}

func NewTransaction(t *types.TxResult) *Transaction {
	return &Transaction{t: t}
}

func (t *Transaction) ID() string {
	return fmt.Sprintf("%d_%d", t.t.Height, t.t.Index)
}

func (t *Transaction) Index() int {
	return int(t.t.Index)
}

func (t *Transaction) BlockHeight() int {
	return int(t.t.Height)
}

func (t *Transaction) GasWanted() int {
	return int(t.t.Response.GasWanted)
}

func (t *Transaction) GasUsed() int {
	return int(t.t.Response.GasUsed)
}

func (t *Transaction) ContentRaw() string {
	return t.t.Tx.String()
}
