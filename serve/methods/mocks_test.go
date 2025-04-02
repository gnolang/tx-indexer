package methods

import (
	"github.com/gnolang/gno/tm2/pkg/amino"
	abci "github.com/gnolang/gno/tm2/pkg/bft/abci/types"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/gno/tm2/pkg/std"
)

func makeTxResult(
	height,
	gasFeeAmount int64,
	gasFeeDenom string,
	gasUsed int64,
) *types.TxResult {
	return &types.TxResult{
		Height: height,
		Index:  0,
		Tx: amino.MustMarshal(std.Tx{
			Fee: std.Fee{
				GasFee: std.Coin{
					Denom:  gasFeeDenom,
					Amount: gasFeeAmount,
				},
			},
		}),
		Response: abci.ResponseDeliverTx{
			GasUsed: gasUsed,
		},
	}
}
