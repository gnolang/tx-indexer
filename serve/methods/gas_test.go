package methods

import (
	"testing"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetGasPricesByBlocks_EmptyTransactions(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name      string
		txResults []*types.TxResult
	}{
		{
			"txs is nil",
			nil,
		},
		{
			"tx is empty",
			make([]*types.TxResult, 0),
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			response, err := GetGasPricesByTxResults(testCase.txResults)

			assert.Nil(t, err)

			require.NotNil(t, response)

			assert.Equal(t, len(response), 0)
		})
	}
}

func TestGetGasPricesByBlocks_Transactions(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name      string
		txResults []*types.TxResult
		results   []GasPrice
	}{
		{
			"single transaction",
			[]*types.TxResult{
				makeTxResult(1, 1, "ugnot", 1),
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
				makeTxResult(1, 1, "ugnot", 1),
				makeTxResult(2, 1, "ugnot", 2),
				makeTxResult(3, 1, "ugnot", 3),
				makeTxResult(4, 1, "ugnot", 4),
			},
			[]GasPrice{
				{
					Denom:   "ugnot",
					High:    1,
					Average: 0.5208333333333333,
					Low:     0.25,
				},
			},
		},
		{
			"variable amounts and coins",
			[]*types.TxResult{
				makeTxResult(1, 1, "ugnot", 1),
				makeTxResult(2, 1, "ugnot", 2),
				makeTxResult(3, 3, "uatom", 3),
				makeTxResult(4, 2, "uatom", 4),
			},
			[]GasPrice{
				{
					Denom:   "ugnot",
					High:    1,
					Average: 0.75,
					Low:     0.5,
				},
				{
					Denom:   "uatom",
					High:    1,
					Average: 0.75,
					Low:     0.5,
				},
			},
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			response, err := GetGasPricesByTxResults(testCase.txResults)

			assert.Nil(t, err)
			require.NotNil(t, response)

			count := 0

			for _, responseItem := range response {
				for _, testCaseResult := range testCase.results {
					if responseItem.Denom != testCaseResult.Denom {
						continue
					}

					assert.Equal(t, responseItem.Denom, testCaseResult.Denom)
					assert.Equal(t, responseItem.High, testCaseResult.High)
					assert.Equal(t, responseItem.Average, testCaseResult.Average)
					assert.Equal(t, responseItem.Low, testCaseResult.Low)

					count++
				}
			}

			assert.Equal(t, count, len(testCase.results))
		})
	}
}
