package tx

import "github.com/gnolang/gno/tm2/pkg/bft/types"

type Storage interface {
	// GetTx returns specified tx from permanent storage
	GetTx(uint64, uint32) (*types.TxResult, error)

	// GetTxByHash fetches the tx using the transaction hash
	GetTxByHash(txHash string) (*types.TxResult, error)
}
