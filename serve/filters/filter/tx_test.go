package filter

import (
	"fmt"
	"testing"

	abci "github.com/gnolang/gno/tm2/pkg/bft/abci/types"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetHashes(t *testing.T) {
	t.Parallel()

	txs := []*types.TxResult{
		{Tx: []byte(`c25dda249cdece9d908cc33adcd16aa05e20290f`)},
		{Tx: []byte(`71ac9eed6a76a285ae035fe84a251d56ae9485a4`)},
		{Tx: []byte(`356a192b7913b04c54574d18c28d46e6395428ab`)},
		{Tx: []byte(`da4b9237bacccdf19c0760cab7aec4a8359010b0`)},
		{Tx: []byte(`77de68daecd823babbb58edb1c8e14d7106e83bb`)},
		{Tx: []byte(`1b6453892473a467d07372d45eb05abc2031647a`)},
		{Tx: []byte(`ac3478d69a3c81fa62e60f5c3696165a4e5e6ac4`)},
		{Tx: []byte(`c1dfd96eea8cc2b62785275bca38ac261256e278`)},
	}

	f := NewTxFilter(TxFilterOption{})

	for _, tx := range txs {
		f.UpdateWith(tx)
	}

	hashes := f.GetHashes()
	require.Len(
		t, hashes, 8,
		fmt.Sprintf("There should be 8 hashes in the filter: %v", len(hashes)),
	)

	for i, hs := range hashes {
		assert.Equal(
			t, txs[i].Tx.Hash(), hs,
			fmt.Sprintf("The hash should match the expected hash: %v", txs[i].Tx.Hash()),
		)
	}
}

func TestApplyFilters(t *testing.T) {
	t.Parallel()

	txs := []*types.TxResult{
		{
			Height: 100,
			Index:  0,
			Tx:     []byte(`sampleTx0`),
			Response: abci.ResponseDeliverTx{
				GasWanted: 1000,
				GasUsed:   900,
				ResponseBase: abci.ResponseBase{
					Data: []byte(`data0`),
				},
			},
		},
		{
			Height: 101,
			Index:  1,
			Tx:     []byte(`sampleTx1`),
			Response: abci.ResponseDeliverTx{
				GasWanted: 1200,
				GasUsed:   1100,
				ResponseBase: abci.ResponseBase{
					Data: []byte(`data1`),
				},
			},
		},
		{
			Height: 102,
			Index:  2,
			Tx:     []byte(`sampleTx2`),
			Response: abci.ResponseDeliverTx{
				GasWanted: 1000,
				GasUsed:   1400,
				ResponseBase: abci.ResponseBase{
					Data: []byte(`data2`),
				},
			},
		},
		{
			Height: 103,
			Index:  3,
			Tx:     []byte(`sampleTx3`),
			Response: abci.ResponseDeliverTx{
				GasWanted: 1200,
				GasUsed:   900,
				ResponseBase: abci.ResponseBase{
					Data: []byte(`data3`),
				},
			},
		},
		{
			Height: 104,
			Index:  4,
			Tx:     []byte(`sampleTx4`),
			Response: abci.ResponseDeliverTx{
				GasWanted: 1100,
				GasUsed:   1000,
				ResponseBase: abci.ResponseBase{
					Data: []byte(`data4`),
				},
			},
		},
	}

	tests := []struct {
		options  TxFilterOption
		name     string
		expected []*types.TxResult
	}{
		{
			name:     "no filter",
			options:  TxFilterOption{},
			expected: txs,
		},
		{
			name: "min gas used is 0",
			options: TxFilterOption{
				GasUsed: &RangeFilterOption{Min: int64Ptr(0), Max: int64Ptr(1000)},
			},
			expected: []*types.TxResult{txs[0], txs[3], txs[4]},
		},
		{
			name: "invalid gas used",
			options: TxFilterOption{
				GasUsed: &RangeFilterOption{Min: int64Ptr(1000), Max: int64Ptr(900)},
			},
			expected: []*types.TxResult{},
		},
		{
			name: "filter by gas wanted 1",
			options: TxFilterOption{
				GasWanted: &RangeFilterOption{Min: int64Ptr(1100), Max: int64Ptr(1200)},
			},
			expected: []*types.TxResult{txs[1], txs[3], txs[4]},
		},
		{
			name: "gas wanted min, max is same value",
			options: TxFilterOption{
				GasWanted: &RangeFilterOption{Min: int64Ptr(1000), Max: int64Ptr(1000)},
			},
			expected: []*types.TxResult{txs[0], txs[2]},
		},
		{
			name: "filter by gas used 2",
			options: TxFilterOption{
				GasUsed: &RangeFilterOption{Min: int64Ptr(900), Max: int64Ptr(1000)},
			},
			expected: []*types.TxResult{txs[0], txs[3], txs[4]},
		},
		{
			name: "gas used min, max is same value",
			options: TxFilterOption{
				GasUsed: &RangeFilterOption{Min: int64Ptr(1000), Max: int64Ptr(1000)},
			},
			expected: []*types.TxResult{txs[4]},
		},
		{
			name: "filter by gas wanted is invalid",
			options: TxFilterOption{
				GasWanted: &RangeFilterOption{Min: int64Ptr(1200), Max: int64Ptr(1100)},
			},
			expected: []*types.TxResult{},
		},
		{
			name: "gas used filter is invalid",
			options: TxFilterOption{
				GasUsed: &RangeFilterOption{Min: int64Ptr(1000), Max: int64Ptr(900)},
			},
			expected: []*types.TxResult{},
		},
		{
			name: "gas limit min value is nil",
			options: TxFilterOption{
				GasLimit: &RangeFilterOption{Min: nil, Max: int64Ptr(1000)},
			},
			expected: []*types.TxResult{txs[0], txs[3], txs[4]},
		},
		{
			name: "gas limit max value is nil",
			options: TxFilterOption{
				GasLimit: &RangeFilterOption{Min: int64Ptr(1100), Max: nil},
			},
			expected: []*types.TxResult{txs[1], txs[2]},
		},
		{
			name: "gas limit range is valid",
			options: TxFilterOption{
				GasLimit: &RangeFilterOption{Min: int64Ptr(900), Max: int64Ptr(1000)},
			},
			expected: []*types.TxResult{txs[0], txs[3], txs[4]},
		},
		{
			name: "gas limit both min and max are nil",
			options: TxFilterOption{
				GasLimit: &RangeFilterOption{Min: nil, Max: nil},
			},
			expected: txs,
		},
		{
			name: "gas limit min is larger than max",
			options: TxFilterOption{
				GasLimit: &RangeFilterOption{Min: int64Ptr(1000), Max: int64Ptr(900)},
			},
			expected: []*types.TxResult{},
		},
		{
			name: "gas used min is nil",
			options: TxFilterOption{
				GasUsed: &RangeFilterOption{Min: nil, Max: int64Ptr(1000)},
			},
			expected: []*types.TxResult{txs[0], txs[3], txs[4]},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := NewTxFilter(tt.options)

			for _, tx := range txs {
				f.UpdateWith(tx)
			}

			changes := f.GetChanges()
			require.Len(
				t, changes, len(tt.expected),
				fmt.Sprintf(
					"There should be one transaction after applying filters: %v",
					len(tt.expected),
				),
			)

			for i, tx := range changes {
				assert.Equal(
					t, *tt.expected[i], tx,
					fmt.Sprintf(
						"The filtered transaction should match the expected transaction: %v",
						tt.expected[i],
					),
				)
			}
		})
	}
}

func TestApplyFiltersWithLargeData(t *testing.T) {
	t.Parallel()

	const txCount = 100000

	txs := make([]*types.TxResult, txCount)

	for i := 0; i < txCount; i++ {
		txs[i] = &types.TxResult{
			Height: int64(i / 10000),
			Index:  uint32(i),
			Tx:     []byte(fmt.Sprintf("sampleTx%d", i)),
			Response: abci.ResponseDeliverTx{
				GasWanted: int64(1000 + i%200),
				GasUsed:   int64(900 + i%100),
				ResponseBase: abci.ResponseBase{
					Data: []byte(fmt.Sprintf("data%d", i)),
				},
			},
		}
	}

	tests := []struct {
		options  TxFilterOption
		name     string
		expected int
	}{
		{
			name:     "no filter",
			options:  TxFilterOption{},
			expected: txCount,
		},
		{
			name: "filter by gas used",
			options: TxFilterOption{
				GasUsed: &RangeFilterOption{Min: int64Ptr(950), Max: int64Ptr(1000)},
			},
			expected: txCount / 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := NewTxFilter(tt.options)

			for _, tx := range txs {
				f.UpdateWith(tx)
			}

			changes := f.GetChanges()
			require.Len(
				t, changes, tt.expected,
				fmt.Sprintf(
					"There should be %d transactions after applying filters. got %d",
					tt.expected, len(changes),
				),
			)
		})
	}
}

func int64Ptr(i int64) *int64 { return &i }
