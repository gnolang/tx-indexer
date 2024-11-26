package methods

import (
	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/gno/tm2/pkg/std"
)

func makeTxResultWithGasFee(gasFeeAmount int64, gasFeeDenom string) *types.TxResult {
	tx := std.Tx{
		Fee: std.Fee{
			GasFee: std.Coin{
				Denom:  gasFeeDenom,
				Amount: gasFeeAmount,
			},
		},
	}

	return &types.TxResult{
		Tx: amino.MustMarshal(tx),
	}
}
