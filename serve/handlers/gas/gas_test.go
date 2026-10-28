package gas

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/gno/tm2/pkg/std"
	"github.com/gnolang/tx-indexer/internal/tally"
	"github.com/gnolang/tx-indexer/serve/deadline"
	"github.com/gnolang/tx-indexer/serve/spec"
	"github.com/gnolang/tx-indexer/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestGetGasPriceHandler_InvalidParams(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name   string
		params []any
	}{
		{
			"invalid param length",
			[]any{1},
		},
		{
			"invalid param type",
			[]any{"totally invalid param type"},
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			h := NewHandler(&mockStorage{})

			response, err := h.GetGasPriceHandler(t.Context(), nil, testCase.params)
			assert.Nil(t, response)

			require.NotNil(t, err)
			assert.Equal(t, spec.InvalidParamsErrorCode, err.Code)
		})
	}
}

// A block range the iterator cannot read in full fails the request: prices
// computed over the blocks read so far would pass part of the range off as
// all of it.
func TestGetGasPriceHandler_IteratorErrors(t *testing.T) {
	t.Parallel()

	errIterator := errors.New("iterator failure")

	testTable := []struct {
		iterator *mockIterator
		name     string
	}{
		{
			&mockIterator{
				nextFn:  func() bool { return true },
				valueFn: func() (*types.Block, error) { return nil, errIterator },
			},
			"a block fails to decode",
		},
		{
			&mockIterator{
				errorFn: func() error { return errIterator },
			},
			"the iteration fails",
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			h := NewHandler(&mockStorage{
				blockIteratorFn: func(uint64, uint64) (storage.Iterator[*types.Block], error) {
					return testCase.iterator, nil
				},
			})

			response, err := h.GetGasPriceHandler(t.Context(), nil, []any{"1", "2"})
			assert.Nil(t, response)

			require.NotNil(t, err)
			assert.Equal(t, spec.ServerErrorCode, err.Code)
			assert.Equal(t, errIterator.Error(), err.Message)
		})
	}
}

// A scan stops once its request's context ends and answers the context's
// cause, instead of reading the rest of the range for a caller that is gone
func TestGetGasPriceHandler_StopsWhenTheContextEnds(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const (
			blocks   = 1_000
			perBlock = time.Millisecond
			bound    = 50 * time.Millisecond
		)

		errBound := errors.New("request bound reached")

		ctx, cancel := context.WithTimeoutCause(t.Context(), bound, errBound)
		defer cancel()

		read := 0
		h := NewHandler(&mockStorage{
			blockIteratorFn: func(uint64, uint64) (storage.Iterator[*types.Block], error) {
				return &mockIterator{
					nextFn: func() bool {
						time.Sleep(perBlock)

						read++

						return read <= blocks
					},
					valueFn: func() (*types.Block, error) { return &types.Block{}, nil },
				}, nil
			},
		})

		start := time.Now()

		response, err := h.GetGasPriceHandler(ctx, nil, []any{"0", "0"})
		assert.Nil(t, response)

		require.NotNil(t, err)
		assert.Equal(t, spec.ServerErrorCode, err.Code)
		assert.Equal(t, errBound.Error(), err.Message)
		assert.Less(t, time.Since(start), 2*bound, "the scan ran past its context")
	})
}

// A scan stopped by the query deadline answers its machine-readable code
// beside the message
func TestGetGasPriceHandler_NamesTheQueryDeadline(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := deadline.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()

		h := NewHandler(&mockStorage{
			blockIteratorFn: func(uint64, uint64) (storage.Iterator[*types.Block], error) {
				return &mockIterator{
					nextFn: func() bool {
						time.Sleep(time.Millisecond)

						return true
					},
					valueFn: func() (*types.Block, error) { return &types.Block{}, nil },
				}, nil
			},
		})

		_, err := h.GetGasPriceHandler(ctx, nil, []any{"0", "0"})

		require.NotNil(t, err)
		assert.Equal(t, "query deadline exceeded (50ms); narrow the block height range", err.Message)
		assert.Equal(t, map[string]any{"code": "QUERY_DEADLINE_EXCEEDED", "queryDeadline": "50ms"}, err.Data)
	})
}

// The deadline can pass once the blocks are read, before the prices are
// computed. The computation checks it too, and the request answers the named
// deadline rather than prices.
func TestGetGasPriceHandler_DeadlinePassesBeforeTheComputation(t *testing.T) {
	t.Parallel()

	const (
		blocks      = 5
		txsPerBlock = 1
	)

	txs := make(types.Txs, 0, txsPerBlock)
	for amount := int64(1); amount <= txsPerBlock; amount++ {
		txs = append(txs, amino.MustMarshal(std.Tx{
			Fee: std.Fee{GasFee: std.Coin{Denom: "ugnot", Amount: amount}},
		}))
	}

	block := &types.Block{Data: types.Data{Txs: txs}}

	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)

	read := 0
	h := NewHandler(&mockStorage{
		blockIteratorFn: func(uint64, uint64) (storage.Iterator[*types.Block], error) {
			return &mockIterator{
				nextFn: func() bool {
					read++
					if read <= blocks {
						return true
					}

					// Every block is read: the deadline passes now, before
					// the prices are computed
					cancel(&deadline.ExceededError{Deadline: 50 * time.Millisecond})

					return false
				},
				valueFn: func() (*types.Block, error) { return block, nil },
			}, nil
		},
	})

	response, err := h.GetGasPriceHandler(ctx, nil, []any{"0", "0"})
	assert.Nil(t, response)

	require.NotNil(t, err)
	assert.Equal(t, "query deadline exceeded (50ms); narrow the block height range", err.Message)
	assert.Equal(t, map[string]any{"code": "QUERY_DEADLINE_EXCEEDED", "queryDeadline": "50ms"}, err.Data)
}

// The blocks the range scan reads count as rows scanned in the tally of the
// request; the gas prices it answers are no rows
func TestGetGasPriceHandler_TalliesTheBlocksItScans(t *testing.T) {
	t.Parallel()

	s, err := storage.NewPebble(t.TempDir(), zap.NewNop())
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, s.Close())
	})

	wb := s.WriteBatch()

	for height := int64(1); height <= 10; height++ {
		require.NoError(t, wb.SetBlock(&types.Block{Header: types.Header{Height: height}}))
	}

	require.NoError(t, wb.Commit())

	rows := &tally.Rows{}

	_, jsonErr := NewHandler(s).GetGasPriceHandler(
		tally.NewContext(context.Background(), rows),
		nil,
		[]any{"3", "7"},
	)
	require.Nil(t, jsonErr)

	assert.Equal(t, int64(5), rows.Scanned())
	assert.Zero(t, rows.Returned())
}
