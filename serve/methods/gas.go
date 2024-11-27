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

// GetGasPricesByBlock calculates the gas price statistics (low, high, average)
// for a single block.
func GetGasPricesByBlock(block *types.Block) ([]*GasPrice, error) {
	blocks := []*types.Block{block}

	return GetGasPricesByBlocks(blocks)
}

// GetGasPricesByBlocks calculates the gas price statistics (low, high, average)
// for multiple blocks.
func GetGasPricesByBlocks(blocks []*types.Block) ([]*GasPrice, error) {
	gasFeeInfoMap := make(map[string]*gasFeeTotalInfo)

	for _, block := range blocks {
		blockGasFeeInfo := calculateGasFeePerBlock(block)

		for denom, gasFeeInfo := range blockGasFeeInfo {
			currentGasFeeInfo := gasFeeInfoMap[denom]
			gasFeeInfoMap[denom] = calculateGasFee(currentGasFeeInfo, gasFeeInfo)
		}
	}

	return calculateGasPrices(gasFeeInfoMap), nil
}

// calculateGasFeePerBlock processes all transactions in a single block to compute
// gas fee statistics (low, high, total amount, total count) for each gas fee denomination.
func calculateGasFeePerBlock(block *types.Block) map[string]*gasFeeTotalInfo {
	gasFeeInfo := make(map[string]*gasFeeTotalInfo)

	for _, t := range block.Txs {
		var stdTx std.Tx
		if err := amino.Unmarshal(t, &stdTx); err != nil {
			continue
		}

		denom := stdTx.Fee.GasFee.Denom
		amount := stdTx.Fee.GasFee.Amount

		info := gasFeeInfo[denom]
		if info == nil {
			info = &gasFeeTotalInfo{}
			gasFeeInfo[denom] = info
		}

		info.Low = min(info.Low, amount)
		info.High = max(info.High, amount)
		info.TotalAmount += amount
		info.TotalCount++
	}

	return gasFeeInfo
}

// calculateGasFee merges the gas fee statistics from a block into the global statistics.
func calculateGasFee(currentInfo *gasFeeTotalInfo, blockInfo *gasFeeTotalInfo) *gasFeeTotalInfo {
	if currentInfo == nil {
		currentInfo = &gasFeeTotalInfo{}
	}

	currentInfo.Low = min(currentInfo.Low, blockInfo.Low)
	currentInfo.High = max(currentInfo.High, blockInfo.High)
	currentInfo.TotalAmount += blockInfo.TotalAmount / blockInfo.TotalCount
	currentInfo.TotalCount++

	return currentInfo
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
			Average: info.TotalAmount / info.TotalCount,
			Denom:   denom,
		})
	}

	return gasPrices
}

// min calculates the smaller of two values, or returns the new value
// if the current value is uninitialized (0).
func min(current, newValue int64) int64 {
	if current == 0 || newValue < current {
		return newValue
	}
	return current
}

// max calculates the larger of two values.
func max(current, newValue int64) int64 {
	if newValue > current {
		return newValue
	}
	return current
}
