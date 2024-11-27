package methods

import (
	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/gno/tm2/pkg/std"
)

func makeBlockWithTxs(height int64, txs []types.Tx) *types.Block {
	return &types.Block{
		Header: types.Header{
			Height: height,
		},
		Data: types.Data{
			Txs: txs,
		},
	}
}

func makeTxResultWithGasFee(gasFeeAmount int64, gasFeeDenom string) types.Tx {
	tx := std.Tx{
		Fee: std.Fee{
			GasFee: std.Coin{
				Denom:  gasFeeDenom,
				Amount: gasFeeAmount,
			},
		},
	}

	return amino.MustMarshal(tx)
}
