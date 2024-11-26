package methods

import (
	"testing"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetGasPricesByTxResults_EmptyTransactions(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name string
		txs  []*types.TxResult
	}{
		{
			"txs is nil",
			nil,
		},
		{
			"tx is empty",
			[]*types.TxResult{},
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			response, err := GetGasPricesByTxResults(testCase.txs)

			assert.Nil(t, err)

			require.NotNil(t, response)

			assert.Equal(t, len(response), 0)
		})
	}
}

func TestGetGasPricesByTxResults_Transactions(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name    string
		txs     []*types.TxResult
		results []GasPrice
	}{
		{
			"single transaction",
			[]*types.TxResult{
				makeTxResultWithGasFee(1, "ugnot"),
			},
			[]GasPrice{
				{
					Denom:   "ugnot",
					High:    1,
					Average: 1,
					Low:     1,
				},
			},
		},
		{
			"variable amount",
			[]*types.TxResult{
				makeTxResultWithGasFee(1, "ugnot"),
				makeTxResultWithGasFee(2, "ugnot"),
				makeTxResultWithGasFee(3, "ugnot"),
				makeTxResultWithGasFee(4, "ugnot"),
			},
			[]GasPrice{
				{
					Denom:   "ugnot",
					High:    4,
					Average: 2,
					Low:     1,
				},
			},
		},
		{
			"variable amounts and coins",
			[]*types.TxResult{
				makeTxResultWithGasFee(1, "ugnot"),
				makeTxResultWithGasFee(2, "ugnot"),
				makeTxResultWithGasFee(3, "uatom"),
				makeTxResultWithGasFee(4, "uatom"),
			},
			[]GasPrice{
				{
					Denom:   "ugnot",
					High:    2,
					Average: 1,
					Low:     1,
				},
				{
					Denom:   "uatom",
					High:    4,
					Average: 3,
					Low:     3,
				},
			},
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			response, err := GetGasPricesByTxResults(testCase.txs)

			assert.Nil(t, err)
			require.NotNil(t, response)

			count := 0

			for _, responseItem := range response {
				for _, testCaseResult := range testCase.results {
					if responseItem.Denom == testCaseResult.Denom {
						assert.Equal(t, responseItem.Denom, testCaseResult.Denom)
						assert.Equal(t, responseItem.High, testCaseResult.High)
						assert.Equal(t, responseItem.Average, testCaseResult.Average)
						assert.Equal(t, responseItem.Low, testCaseResult.Low)

						count++
					}
				}
			}

			assert.Equal(t, count, len(testCase.results))
		})
	}
}
