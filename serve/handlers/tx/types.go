package tx

import (
	"context"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

type Storage interface {
	// GetTx returns specified tx from permanent storage
	GetTx(ctx context.Context, blockNum uint64, index uint32) (*types.TxResult, error)

	// GetTxByHash fetches the tx using the transaction hash
	GetTxByHash(ctx context.Context, txHash string) (*types.TxResult, error)
}
