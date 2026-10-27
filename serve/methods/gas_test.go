package methods

import (
	"context"
	"errors"
	"testing"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnolang/tx-indexer/internal/contexttest"
)

func TestGetGasPricesByBlocks_EmptyTransactions(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name   string
		blocks []*types.Block
	}{
		{
			"txs is nil",
			nil,
		},
		{
			"tx is empty",
			make([]*types.Block, 0),
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			response, err := GetGasPricesByBlocks(t.Context(), testCase.blocks)

			assert.Nil(t, err)

			require.NotNil(t, response)

			assert.Equal(t, len(response), 0)
		})
	}
}

func TestGetGasPricesByBlocks_Transactions(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name    string
		blocks  []*types.Block
		results []GasPrice
	}{
		{
			"single transaction",
			[]*types.Block{
				makeBlockWithTxs(1,
					[]types.Tx{
						makeTxResultWithGasFee(1, "ugnot"),
					},
				),
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
			[]*types.Block{
				makeBlockWithTxs(1,
					[]types.Tx{
						makeTxResultWithGasFee(1, "ugnot"),
						makeTxResultWithGasFee(2, "ugnot"),
						makeTxResultWithGasFee(3, "ugnot"),
						makeTxResultWithGasFee(4, "ugnot"),
					},
				),
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
			[]*types.Block{
				makeBlockWithTxs(1,
					[]types.Tx{
						makeTxResultWithGasFee(1, "ugnot"),
						makeTxResultWithGasFee(2, "ugnot"),
						makeTxResultWithGasFee(3, "uatom"),
						makeTxResultWithGasFee(4, "uatom"),
					},
				),
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
		{
			"calculate the average value per block",
			[]*types.Block{
				makeBlockWithTxs(1,
					[]types.Tx{
						makeTxResultWithGasFee(1, "ugnot"),
					},
				),
				makeBlockWithTxs(2,
					[]types.Tx{
						makeTxResultWithGasFee(10, "ugnot"),
						makeTxResultWithGasFee(10, "ugnot"),
						makeTxResultWithGasFee(10, "ugnot"),
					},
				),
			},
			[]GasPrice{
				{
					Denom:   "ugnot",
					High:    10,
					Average: 5,
					Low:     1,
				},
			},
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			response, err := GetGasPricesByBlocks(t.Context(), testCase.blocks)

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

// The computation decodes every transaction of every block, so over a wide
// range it can cost more than the reads: it stops at the first block after its
// context ends, and answers the context's cause
func TestGetGasPricesByBlocks_StopsWhenTheContextEnds(t *testing.T) {
	t.Parallel()

	const budget = 2

	errEnded := errors.New("request ended")

	blocks := make([]*types.Block, 0, 5)
	for height := int64(1); height <= 5; height++ {
		blocks = append(blocks, makeBlockWithTxs(height, []types.Tx{makeTxResultWithGasFee(height, "ugnot")}))
	}

	ctx := &contexttest.Countdown{
		Context:  context.Background(),
		EndsWith: errEnded,
		Budget:   budget,
	}

	prices, err := GetGasPricesByBlocks(ctx, blocks)

	assert.Nil(t, prices)
	require.ErrorIs(t, err, errEnded)

	// One check per block until the context ends, plus the ask for its cause
	assert.LessOrEqual(t, ctx.Asked, budget+2, "the computation ran on after its context ended")
}
