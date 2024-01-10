package tx

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/serve/spec"
	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			[]any{1},
		},
		{
			"invalid base64 type",
			[]any{"totally base64"},
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
			txHash   = []byte("random")
			txHash64 = base64.StdEncoding.EncodeToString(txHash)

			mockStorage = &mockStorage{
				getTxFn: func(hash []byte) (*types.TxResult, error) {
					require.Equal(t, txHash, hash)

					return nil, storageErrors.ErrNotFound
				},
			}
		)

		h := NewHandler(mockStorage)

		response, err := h.GetTxHandler(nil, []any{txHash64})

		// This is a special case
		assert.Nil(t, response)
		assert.Nil(t, err)
	})

	t.Run("random fetch error", func(t *testing.T) {
		t.Parallel()

		var (
			txHash   = []byte("random")
			txHash64 = base64.StdEncoding.EncodeToString(txHash)

			fetchErr = errors.New("random error")

			mockStorage = &mockStorage{
				getTxFn: func(_ []byte) (*types.TxResult, error) {
					return nil, fetchErr
				},
			}
		)

		h := NewHandler(mockStorage)

		response, err := h.GetTxHandler(nil, []any{txHash64})
		assert.Nil(t, response)

		// Make sure the error is populated
		require.NotNil(t, err)

		assert.Equal(t, spec.ServerErrorCode, err.Code)
		assert.Equal(t, fetchErr.Error(), err.Message)
	})

	t.Run("block found in storage", func(t *testing.T) {
		t.Parallel()

		var (
			txHash   = []byte("random")
			txHash64 = base64.StdEncoding.EncodeToString(txHash)

			txResult = &types.TxResult{
				Height: 10,
			}

			mockStorage = &mockStorage{
				getTxFn: func(hash []byte) (*types.TxResult, error) {
					require.Equal(t, txHash, hash)

					return txResult, nil
				},
			}
		)

		h := NewHandler(mockStorage)

		responseRaw, err := h.GetTxHandler(nil, []any{txHash64})
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
