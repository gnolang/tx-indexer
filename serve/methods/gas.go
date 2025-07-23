package methods

import (
	"fmt"

	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/gno/tm2/pkg/std"
)

type gasFeeTotalInfo struct {
	Low         float64
	High        float64
	TotalAmount float64
	TotalCount  int64
}

// GetGasPricesByTxResult calculates the gas price statistics (low, high, average)
// for a single txResult.
func GetGasPricesByTxResult(txResult *types.TxResult) ([]*GasPrice, error) {
	txResults := []*types.TxResult{txResult}

	return GetGasPricesByTxResults(txResults)
}

// GetGasPricesByTxResults calculates the gas price statistics (low, high, average)
// for multiple txResults.
func GetGasPricesByTxResults(txResults []*types.TxResult) ([]*GasPrice, error) {
	gasFeeInfoMap := make(map[string]*gasFeeTotalInfo)

	txResultGasFeeInfo := calculateGasFeePerTxResults(txResults)

	for denom, gasFeeInfo := range txResultGasFeeInfo {
		_, exists := gasFeeInfoMap[denom]
		if !exists {
			gasFeeInfoMap[denom] = &gasFeeTotalInfo{}
		}

		err := modifyAggregatedInfo(gasFeeInfoMap[denom], gasFeeInfo)
		if err != nil {
			return nil, err
		}
	}

	return calculateGasPrices(gasFeeInfoMap), nil
}

// calculateGasFeePerTxResult processes all transactions in a single txResult to compute
// gas fee statistics (low, high, total amount, total count) for each gas fee denomination.
func calculateGasFeePerTxResults(txResults []*types.TxResult) map[string]*gasFeeTotalInfo {
	gasFeeInfo := make(map[string]*gasFeeTotalInfo)

	for _, t := range txResults {
		if t.Response.IsErr() ||
			t.Response.GasUsed == 0 ||
			t.Height <= 0 {
			continue
		}

		var stdTx std.Tx
		if err := amino.Unmarshal(t.Tx, &stdTx); err != nil {
			continue
		}

		denom := stdTx.Fee.GasFee.Denom
		gasFeeAmount := stdTx.Fee.GasFee.Amount
		GasUsedAmount := t.Response.GasUsed

		// Calculate the gas price (gasFee / gasUsed)
		gasPrice := float64(gasFeeAmount) / float64(GasUsedAmount)

		info := gasFeeInfo[denom]
		if info == nil {
			info = &gasFeeTotalInfo{}
			gasFeeInfo[denom] = info
		}

		info.Low = minWithDefault(info.Low, gasPrice)
		info.High = max(info.High, gasPrice)
		info.TotalAmount += gasPrice
		info.TotalCount++
	}

	return gasFeeInfo
}

// modifyAggregatedInfo updates the aggregated gas fee statistics by merging the txResult's statistics.
func modifyAggregatedInfo(currentInfo, txResultInfo *gasFeeTotalInfo) error {
	if currentInfo == nil {
		return fmt.Errorf("not initialized aggregated data")
	}

	currentInfo.Low = minWithDefault(currentInfo.Low, txResultInfo.Low)
	currentInfo.High = max(currentInfo.High, txResultInfo.High)
	currentInfo.TotalAmount += txResultInfo.TotalAmount / float64(txResultInfo.TotalCount)
	currentInfo.TotalCount++

	return nil
}

// calculateGasPrices generates the final gas price statistics (low, high, average)
func calculateGasPrices(gasFeeInfoMap map[string]*gasFeeTotalInfo) []*GasPrice {
	gasPrices := make([]*GasPrice, 0, len(gasFeeInfoMap))

	for denom, info := range gasFeeInfoMap {
		if info.TotalCount == 0 {
			continue
		}

		gasPrices = append(gasPrices, &GasPrice{
			High:    info.High,
			Low:     info.Low,
			Average: info.TotalAmount / float64(info.TotalCount),
			Denom:   denom,
		})
	}

	return gasPrices
}

// min calculates the smaller of two values, or returns the new value
// if the current value is uninitialized (0).
func minWithDefault(current, newValue float64) float64 {
	if current <= 0 {
		return newValue
	}

	return min(current, newValue)
}
