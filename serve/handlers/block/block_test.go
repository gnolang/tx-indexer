package block

import (
	"encoding/base64"
	"errors"
	"strconv"
	"testing"

	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnolang/tx-indexer/internal/tally"
	"github.com/gnolang/tx-indexer/serve/spec"
	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
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
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			h := NewHandler(&mockStorage{})

			response, err := h.GetBlockHandler(t.Context(), nil, testCase.params)
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
			getBlockFn: func(_ uint64) (*types.Block, error) {
				return nil, storageErrors.ErrNotFound
			},
		}

		h := NewHandler(mockStorage)

		response, err := h.GetBlockHandler(t.Context(), nil, []any{"1"})

		// This is a special case
		assert.Nil(t, response)
		assert.Nil(t, err)
	})

	t.Run("random fetch error", func(t *testing.T) {
		t.Parallel()

		var (
			fetchErr = errors.New("random error")

			mockStorage = &mockStorage{
				getBlockFn: func(_ uint64) (*types.Block, error) {
					return nil, fetchErr
				},
			}
		)

		h := NewHandler(mockStorage)

		response, err := h.GetBlockHandler(t.Context(), nil, []any{"1"})
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
				getBlockFn: func(num uint64) (*types.Block, error) {
					require.EqualValues(t, blockNum, num)

					return block, nil
				},
			}
		)

		h := NewHandler(mockStorage)

		responseRaw, err := h.GetBlockHandler(t.Context(), nil, []any{strconv.FormatInt(blockNum, 10)})
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

// A block read from the storage counts as one row scanned and one answered in
// the tally of its request; a missing one as none
func TestGetBlock_TalliesTheRowItReads(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		block *types.Block
		name  string
		want  int64
	}{
		{&types.Block{Header: types.Header{Height: 1}}, "found", 1},
		{nil, "missing", 0},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			h := NewHandler(&mockStorage{
				getBlockFn: func(uint64) (*types.Block, error) {
					if testCase.block == nil {
						return nil, storageErrors.ErrNotFound
					}

					return testCase.block, nil
				},
			})

			rows := &tally.Rows{}

			_, err := h.GetBlockHandler(tally.NewContext(t.Context(), rows), nil, []any{"1"})
			require.Nil(t, err)

			assert.Equal(t, testCase.want, rows.Scanned())
			assert.Equal(t, testCase.want, rows.Returned())
		})
	}
}
