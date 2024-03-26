package filter

import (
	"fmt"
	"testing"
	"time"

	abci "github.com/gnolang/gno/tm2/pkg/bft/abci/types"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetHashes(t *testing.T) {
	t.Parallel()

	var txs = []*types.TxResult{
		{Tx: []byte(`c25dda249cdece9d908cc33adcd16aa05e20290f`)},
		{Tx: []byte(`71ac9eed6a76a285ae035fe84a251d56ae9485a4`)},
		{Tx: []byte(`356a192b7913b04c54574d18c28d46e6395428ab`)},
		{Tx: []byte(`da4b9237bacccdf19c0760cab7aec4a8359010b0`)},
		{Tx: []byte(`77de68daecd823babbb58edb1c8e14d7106e83bb`)},
		{Tx: []byte(`1b6453892473a467d07372d45eb05abc2031647a`)},
		{Tx: []byte(`ac3478d69a3c81fa62e60f5c3696165a4e5e6ac4`)},
		{Tx: []byte(`c1dfd96eea8cc2b62785275bca38ac261256e278`)},
	}

	f := NewTxFilter(FilterOptions{})
	for _, tx := range txs {
		f.UpdateWithTx(tx)
	}

	hashes := f.GetHashes()
	require.Len(t, hashes, 8, fmt.Sprintf("There should be 8 hashes in the filter: %v", len(hashes)))
	for i, hs := range hashes {
		assert.Equal(t, txs[i].Tx.Hash(), hs, fmt.Sprintf("The hash should match the expected hash: %v", txs[i].Tx.Hash()))
	}
}

func TestApplyFilters(t *testing.T) {
	t.Parallel()

	var txs = []*types.TxResult{
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
		name     string
		options  FilterOptions
		expected []*types.TxResult
	}{
		{
			name:     "no filter",
			options:  FilterOptions{},
			expected: txs,
		},
		{ // took 34.5µs
			name: "filter by index",
			options: FilterOptions{
				Index: 1,
			},
			expected: []*types.TxResult{txs[1]},
		},
		{ // took 29.583µs
			name: "filter by height and gas used",
			options: FilterOptions{
				Height:  100,
				GasUsed: struct{ Min, Max int64 }{900, 1000},
			},
			expected: []*types.TxResult{txs[0]},
		},
		{ // took 37.292µs
			name: "filter by gas wanted 1",
			options: FilterOptions{
				GasWanted: struct{ Min, Max int64 }{1100, 1200},
			},
			expected: []*types.TxResult{txs[1], txs[3], txs[4]},
		},
		{ // took 36.583µs
            name: "filter by gas used 2",
            options: FilterOptions{
                GasUsed: struct{ Min, Max int64 }{900, 1000},
            },
            expected: []*types.TxResult{txs[0], txs[3], txs[4]},
        },
        { // took 15.417µs
            name: "filter by gas wanted is invalid",
            options: FilterOptions{
                GasWanted: struct{ Min, Max int64 }{1200, 1100},
            },
            expected: []*types.TxResult{},
        },
        { // took 15.166µs
            name: "gas used filter is invalid",
            options: FilterOptions{
                GasUsed: struct{ Min, Max int64 }{1000, 900},
            },
            expected: []*types.TxResult{},
        },
        { // took 36.834µs
            name: "use all filters",
            options: FilterOptions{
                Height:  100,
                Index:   0,
                GasUsed: struct{ Min, Max int64 }{900, 1000},
                GasWanted: struct{ Min, Max int64 }{1000, 1100},
            },
            expected: []*types.TxResult{txs[0]},
        },
        { // took 27.167µs
            name: "use all filters but sequence is flipped",
            options: FilterOptions{
                GasWanted: struct{ Min, Max int64 }{1000, 1100},
                GasUsed: struct{ Min, Max int64 }{900, 1000},
                Index:   0,
                Height:  100,
            },
			expected: []*types.TxResult{txs[0]},
        },
	}

	for _, tt := range tests {
		start := time.Now()
		t.Run(tt.name, func(t *testing.T) {
			f := NewTxFilter(tt.options)
			for _, tx := range txs {
				f.UpdateWithTx(tx)
			}

			filtered := f.Apply()
			require.Len(t, filtered, len(tt.expected), fmt.Sprintf("There should be one transaction after applying filters: %v", len(tt.expected)))
			for i, tx := range filtered {
				assert.Equal(t, tt.expected[i], tx, fmt.Sprintf("The filtered transaction should match the expected transaction: %v", tt.expected[i]))
			}

			fmt.Printf("took %v\n", time.Since(start))
		})
	}
}
