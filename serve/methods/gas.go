package methods

import (
	"fmt"

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
			_, exists := gasFeeInfoMap[denom]
			if !exists {
				gasFeeInfoMap[denom] = &gasFeeTotalInfo{}
			}

			err := modifyAggregatedInfo(gasFeeInfoMap[denom], gasFeeInfo)
			if err != nil {
				return nil, err
			}
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

		info.Low = minWithDefault(info.Low, amount)
		info.High = max(info.High, amount)
		info.TotalAmount += amount
		info.TotalCount++
	}

	return gasFeeInfo
}

// modifyAggregatedInfo updates the aggregated gas fee statistics by merging the block's statistics.
func modifyAggregatedInfo(currentInfo, blockInfo *gasFeeTotalInfo) error {
	if currentInfo == nil {
		return fmt.Errorf("not initialized aggregated data")
	}

	currentInfo.Low = minWithDefault(currentInfo.Low, blockInfo.Low)
	currentInfo.High = max(currentInfo.High, blockInfo.High)
	currentInfo.TotalAmount += blockInfo.TotalAmount / blockInfo.TotalCount
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
			Average: info.TotalAmount / info.TotalCount,
			Denom:   denom,
		})
	}

	return gasPrices
}

// min calculates the smaller of two values, or returns the new value
// if the current value is uninitialized (0).
func minWithDefault(current, newValue int64) int64 {
	if current <= 0 {
		return newValue
	}

	return min(current, newValue)
}
