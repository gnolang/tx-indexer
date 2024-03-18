package tx

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnolang/tx-indexer/serve/spec"
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
			[]any{1, 2, 3},
		},
		{
			"invalid param type",
			[]any{"totally invalid param type"},
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			h := NewHandler(&mockStorage{})

			response, err := h.GetTxHandler(nil, testCase.params)
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

		response, err := h.GetTxHandler(nil, []any{blockNum, txIndex})

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

		response, err := h.GetTxHandler(nil, []any{blockNum, txIndex})
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

		responseRaw, err := h.GetTxHandler(nil, []any{blockNum, txIndex})
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

		responseRaw, err := h.GetTxByHashHandler(nil, []any{hash})
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
