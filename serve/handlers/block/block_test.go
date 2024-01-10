package block

import (
	"encoding/base64"
	"errors"
	"math"
	"strconv"
	"testing"

	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/serve/spec"
	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetBlock_InvalidParams(t *testing.T) {
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
			"invalid int64 type",
			[]any{strconv.FormatUint(math.MaxUint64, 10)},
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			h := NewHandler(&mockStorage{})

			response, err := h.GetBlockHandler(nil, testCase.params)
			assert.Nil(t, response)

			require.NotNil(t, err)

			assert.Equal(t, spec.InvalidParamsErrorCode, err.Code)
		})
	}
}

func TestGetBlock_Handler(t *testing.T) {
	t.Parallel()

	t.Run("block not found", func(t *testing.T) {
		t.Parallel()

		mockStorage := &mockStorage{
			getBlockFn: func(_ int64) (*types.Block, error) {
				return nil, storageErrors.ErrNotFound
			},
		}

		h := NewHandler(mockStorage)

		response, err := h.GetBlockHandler(nil, []any{"1"})

		// This is a special case
		assert.Nil(t, response)
		assert.Nil(t, err)
	})

	t.Run("random fetch error", func(t *testing.T) {
		t.Parallel()

		var (
			fetchErr = errors.New("random error")

			mockStorage = &mockStorage{
				getBlockFn: func(_ int64) (*types.Block, error) {
					return nil, fetchErr
				},
			}
		)

		h := NewHandler(mockStorage)

		response, err := h.GetBlockHandler(nil, []any{"1"})
		assert.Nil(t, response)

		// Make sure the error is populated
		require.NotNil(t, err)

		assert.Equal(t, spec.ServerErrorCode, err.Code)
		assert.Equal(t, fetchErr.Error(), err.Message)
	})

	t.Run("block found in storage", func(t *testing.T) {
		t.Parallel()

		var (
			blockNum = int64(10)

			block = &types.Block{
				Header: types.Header{
					Height: blockNum,
				},
			}

			mockStorage = &mockStorage{
				getBlockFn: func(num int64) (*types.Block, error) {
					require.EqualValues(t, blockNum, num)

					return block, nil
				},
			}
		)

		h := NewHandler(mockStorage)

		responseRaw, err := h.GetBlockHandler(nil, []any{strconv.FormatInt(blockNum, 10)})
		require.Nil(t, err)

		require.NotNil(t, responseRaw)

		// Make sure the response is valid (base64 + amino)
		response, ok := responseRaw.(string)
		require.True(t, ok)

		// Decode from base64
		encodedBlock, decodeErr := base64.StdEncoding.DecodeString(response)
		require.Nil(t, decodeErr)

		// Decode from amino binary
		var decodedBlock types.Block

		require.NoError(t, amino.Unmarshal(encodedBlock, &decodedBlock))

		assert.Equal(t, block, &decodedBlock)
	})
}
