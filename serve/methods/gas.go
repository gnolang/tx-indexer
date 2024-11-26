package methods

import (
	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/gno/tm2/pkg/std"
)

type gasFeeTotalInfo struct {
	Low         int64
	High        int64
	TotalAmount int64
	TotalCount  int64
}

func GetGasPricesByTxResults(txs []*types.TxResult) ([]*GasPrice, error) {
	gasFeeInfoMap := make(map[string]*gasFeeTotalInfo)

	for _, t := range txs {
		var stdTx std.Tx
		if err := amino.Unmarshal(t.Tx, &stdTx); err != nil {
			continue
		}

		gasFeeDenom := stdTx.Fee.GasFee.Denom
		gasFeeAmount := stdTx.Fee.GasFee.Amount

		if _, exists := gasFeeInfoMap[gasFeeDenom]; !exists {
			gasFeeInfoMap[gasFeeDenom] = &gasFeeTotalInfo{}
		}

		if gasFeeInfoMap[gasFeeDenom].Low == 0 || gasFeeInfoMap[gasFeeDenom].Low > gasFeeAmount {
			gasFeeInfoMap[gasFeeDenom].Low = gasFeeAmount
		}

		if gasFeeInfoMap[gasFeeDenom].High == 0 || gasFeeInfoMap[gasFeeDenom].High < gasFeeAmount {
			gasFeeInfoMap[gasFeeDenom].High = gasFeeAmount
		}

		gasFeeInfoMap[gasFeeDenom].TotalAmount += gasFeeAmount
		gasFeeInfoMap[gasFeeDenom].TotalCount++
	}

	gasPrices := make([]*GasPrice, 0)

	for denom, gasFeeInfo := range gasFeeInfoMap {
		if gasFeeInfo.TotalCount == 0 {
			continue
		}

		average := gasFeeInfo.TotalAmount / gasFeeInfo.TotalCount

		gasPrices = append(gasPrices, &GasPrice{
			High:    gasFeeInfo.High,
			Low:     gasFeeInfo.Low,
			Average: average,
			Denom:   denom,
		})
	}

	return gasPrices, nil
}
