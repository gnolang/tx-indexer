package tx

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/gnolang/tx-indexer/internal/tally"
	"github.com/gnolang/tx-indexer/serve/spec"
	"github.com/gnolang/tx-indexer/storage"
	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
)

func TestGetTx_InvalidParams(t *testing.T) {
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

			response, err := h.GetTxHandler(t.Context(), nil, testCase.params)
			assert.Nil(t, response)

			require.NotNil(t, err)

			assert.Equal(t, spec.InvalidParamsErrorCode, err.Code)
		})
	}
}

func TestGetBlock_Handler(t *testing.T) {
	t.Parallel()

	t.Run("tx not found", func(t *testing.T) {
		t.Parallel()

		var (
			blockNum = uint64(42)
			txIndex  = uint32(42)

			mockStorage = &mockStorage{
				getTxFn: func(bn uint64, ti uint32) (*types.TxResult, error) {
					require.Equal(t, blockNum, bn)
					require.Equal(t, txIndex, ti)

					return nil, storageErrors.ErrNotFound
				},
			}
		)

		h := NewHandler(mockStorage)

		response, err := h.GetTxHandler(t.Context(), nil, []any{blockNum, txIndex})

		// This is a special case
		assert.Nil(t, response)
		assert.Nil(t, err)
	})

	t.Run("random fetch error", func(t *testing.T) {
		t.Parallel()

		var (
			blockNum = uint64(42)
			txIndex  = uint32(42)

			fetchErr = errors.New("random error")

			mockStorage = &mockStorage{
				getTxFn: func(_ uint64, _ uint32) (*types.TxResult, error) {
					return nil, fetchErr
				},
			}
		)

		h := NewHandler(mockStorage)

		response, err := h.GetTxHandler(t.Context(), nil, []any{blockNum, txIndex})
		assert.Nil(t, response)

		// Make sure the error is populated
		require.NotNil(t, err)

		assert.Equal(t, spec.ServerErrorCode, err.Code)
		assert.Equal(t, fetchErr.Error(), err.Message)
	})

	t.Run("block found in storage", func(t *testing.T) {
		t.Parallel()

		var (
			blockNum = uint64(42)
			txIndex  = uint32(42)

			txResult = &types.TxResult{
				Height: 10,
			}

			mockStorage = &mockStorage{
				getTxFn: func(bn uint64, ti uint32) (*types.TxResult, error) {
					require.Equal(t, blockNum, bn)
					require.Equal(t, txIndex, ti)

					return txResult, nil
				},
			}
		)

		h := NewHandler(mockStorage)

		responseRaw, err := h.GetTxHandler(t.Context(), nil, []any{blockNum, txIndex})
		require.Nil(t, err)

		require.NotNil(t, responseRaw)

		// Make sure the response is valid (base64 + amino)
		response, ok := responseRaw.(string)
		require.True(t, ok)

		// Decode from base64
		encodedTxResult, decodeErr := base64.StdEncoding.DecodeString(response)
		require.Nil(t, decodeErr)

		// Decode from amino binary
		var decodedTxResult types.TxResult

		require.NoError(t, amino.Unmarshal(encodedTxResult, &decodedTxResult))

		assert.Equal(t, txResult, &decodedTxResult)
	})

	t.Run("block found in storage by hash", func(t *testing.T) {
		t.Parallel()

		var (
			hash = "hash"

			txResult = &types.TxResult{
				Height: 10,
			}

			mockStorage = &mockStorage{
				getTxHashFn: func(s string) (*types.TxResult, error) {
					require.Equal(t, hash, s)

					return txResult, nil
				},
			}
		)

		h := NewHandler(mockStorage)

		responseRaw, err := h.GetTxByHashHandler(t.Context(), nil, []any{hash})
		require.Nil(t, err)

		require.NotNil(t, responseRaw)

		// Make sure the response is valid (base64 + amino)
		response, ok := responseRaw.(string)
		require.True(t, ok)

		// Decode from base64
		encodedTxResult, decodeErr := base64.StdEncoding.DecodeString(response)
		require.Nil(t, decodeErr)

		// Decode from amino binary
		var decodedTxResult types.TxResult

		require.NoError(t, amino.Unmarshal(encodedTxResult, &decodedTxResult))

		assert.Equal(t, txResult, &decodedTxResult)
	})
}

// A transaction lookup, by height and index or by hash, counts as one row
// scanned in the tally of its request, whether it finds the transaction or
// not: a miss reads the storage all the same. Only a transaction found counts
// as a row answered.
func TestGetTx_TalliesTheRowItReads(t *testing.T) {
	t.Parallel()

	s, err := storage.NewPebble(t.TempDir(), zap.NewNop())
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, s.Close())
	})

	stored := types.Tx{1}

	wb := s.WriteBatch()
	require.NoError(t, wb.SetTx(&types.TxResult{Height: 1, Tx: stored}))
	require.NoError(t, wb.Commit())

	byHeight := func(height string) func(h *Handler, ctx context.Context) *spec.BaseJSONError {
		return func(h *Handler, ctx context.Context) *spec.BaseJSONError {
			_, jsonErr := h.GetTxHandler(ctx, nil, []any{height, "0"})

			return jsonErr
		}
	}

	byHash := func(hash string) func(h *Handler, ctx context.Context) *spec.BaseJSONError {
		return func(h *Handler, ctx context.Context) *spec.BaseJSONError {
			_, jsonErr := h.GetTxByHashHandler(ctx, nil, []any{hash})

			return jsonErr
		}
	}

	testTable := []struct {
		handle   func(h *Handler, ctx context.Context) *spec.BaseJSONError
		name     string
		returned int64
	}{
		{byHeight("1"), "found by height and index", 1},
		{byHeight("9"), "missing by height and index", 0},
		{byHash(base64.StdEncoding.EncodeToString(stored.Hash())), "found by hash", 1},
		{byHash("unknown"), "missing by hash", 0},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rows := &tally.Rows{}

			require.Nil(t, testCase.handle(NewHandler(s), tally.NewContext(t.Context(), rows)))

			assert.Equal(t, int64(1), rows.Scanned())
			assert.Equal(t, testCase.returned, rows.Returned())
		})
	}
}
