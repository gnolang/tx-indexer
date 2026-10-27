package gas

import (
	"errors"
	"testing"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/serve/spec"
	"github.com/gnolang/tx-indexer/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

			response, err := h.GetGasPriceHandler(nil, testCase.params)
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

			response, err := h.GetGasPriceHandler(nil, []any{"1", "2"})
			assert.Nil(t, response)

			require.NotNil(t, err)
			assert.Equal(t, spec.ServerErrorCode, err.Code)
			assert.Equal(t, errIterator.Error(), err.Message)
		})
	}
}
