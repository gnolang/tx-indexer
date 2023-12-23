package tx

import (
	"context"
	"errors"
	"testing"

	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeFetcher_FetchTransactions_Invalid(t *testing.T) {
	t.Parallel()

	t.Run("unable to fetch latest storage transaction", func(t *testing.T) {
		t.Parallel()

		var (
			fetchErr = errors.New("unable to fetch local db data")

			mockStorage = &mockStorage{
				getLatestTxFn: func(_ context.Context) (*types.TxResult, error) {
					return nil, fetchErr
				},
			}
		)

		// Create the fetcher
		f := NewNodeFetcher(mockStorage, &mockClient{})

		assert.ErrorIs(
			t,
			f.FetchTransactions(context.Background()),
			fetchErr,
		)
	})

	t.Run("unable to get latest block height", func(t *testing.T) {
		t.Parallel()

		var (
			fetchErr = errors.New("unable to get block height")

			mockStorage = &mockStorage{
				getLatestTxFn: func(_ context.Context) (*types.TxResult, error) {
					return &types.TxResult{
						Height: 0,
					}, nil
				},
			}

			mockClient = &mockClient{
				getLatestBlockNumberFn: func() (int64, error) {
					return 0, fetchErr
				},
			}
		)

		// Create the fetcher
		f := NewNodeFetcher(mockStorage, mockClient)

		assert.ErrorIs(
			t,
			f.FetchTransactions(context.Background()),
			fetchErr,
		)
	})

	t.Run("unable to get block data", func(t *testing.T) {
		t.Parallel()

		var (
			blockNum = int64(10)
			fetchErr = errors.New("unable to get block data")

			mockStorage = &mockStorage{
				getLatestTxFn: func(_ context.Context) (*types.TxResult, error) {
					return &types.TxResult{
						Height: blockNum - 1, // to trigger a fetch
					}, nil
				},
			}

			mockClient = &mockClient{
				getLatestBlockNumberFn: func() (int64, error) {
					return blockNum, nil
				},
				getBlockFn: func(num int64) (*core_types.ResultBlock, error) {
					// Sanity check
					require.Equal(t, num, blockNum)

					return nil, fetchErr
				},
			}
		)

		// Create the fetcher
		f := NewNodeFetcher(mockStorage, mockClient)

		assert.ErrorIs(
			t,
			f.FetchTransactions(context.Background()),
			fetchErr,
		)
	})

	t.Run("unable to get block results", func(t *testing.T) {
		t.Parallel()

		var (
			blockNum = int64(10)
			fetchErr = errors.New("unable to get block results")

			mockStorage = &mockStorage{
				getLatestTxFn: func(_ context.Context) (*types.TxResult, error) {
					return &types.TxResult{
						Height: blockNum - 1, // to trigger a fetch
					}, nil
				},
			}

			mockClient = &mockClient{
				getLatestBlockNumberFn: func() (int64, error) {
					return blockNum, nil
				},
				getBlockFn: func(num int64) (*core_types.ResultBlock, error) {
					// Sanity check
					require.Equal(t, num, blockNum)

					return &core_types.ResultBlock{
						BlockMeta: nil,
						Block: &types.Block{
							Header: types.Header{
								NumTxs: 1, // > 0
							},
						},
					}, nil
				},
				getBlockResultsFn: func(num int64) (*core_types.ResultBlockResults, error) {
					// Sanity check
					require.Equal(t, num, blockNum)

					return nil, fetchErr
				},
			}
		)

		// Create the fetcher
		f := NewNodeFetcher(mockStorage, mockClient)

		assert.ErrorIs(
			t,
			f.FetchTransactions(context.Background()),
			fetchErr,
		)
	})
}

func TestNodeFetcher_FetchTransactions_Valid(t *testing.T) {
	t.Parallel()

	t.Run("simple chain catchup", func(t *testing.T) {
		t.Parallel()

	})

	t.Run("chain is caught up, listen for blocks", func(t *testing.T) {
		t.Parallel()

	})
}
