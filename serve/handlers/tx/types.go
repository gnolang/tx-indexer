package tx

import "github.com/gnolang/gno/tm2/pkg/bft/types"

type Storage interface {
	// GetTx returns specified tx from permanent storage
	GetTx(hash []byte) (*types.TxResult, error)
}
