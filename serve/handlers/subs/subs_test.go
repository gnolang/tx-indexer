package subs

import (
	"context"
	"testing"

	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/serve/filters"
	"github.com/gnolang/tx-indexer/serve/filters/filter"
	"github.com/gnolang/tx-indexer/serve/filters/mocks"
	"github.com/gnolang/tx-indexer/serve/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBlockFilter_InvalidParams(t *testing.T) {
	t.Parallel()

	t.Run("invalid param length", func(t *testing.T) {
		t.Parallel()

		h := NewHandler(nil, nil)

		response, err := h.NewBlockFilterHandler(nil, []any{1, 2, 3})
		assert.Nil(t, response)

		require.NotNil(t, err)

		assert.Equal(t, spec.InvalidParamsErrorCode, err.Code)
	})
}

func TestNewBlockFilter_Valid(t *testing.T) {
	t.Parallel()

	fm := filters.NewFilterManager(
		context.Background(),
		&mocks.MockStorage{},
		&mocks.MockEvents{
			SubscribeFn: func(_ []events.Type) *events.Subscription {
				return &events.Subscription{}
			},
		},
	)

	h := NewHandler(fm, nil)

	responseRaw, err := h.NewBlockFilterHandler(nil, []any{})
	require.Nil(t, err)

	response, ok := responseRaw.(string)
	require.True(t, ok)

	// Make sure an ID is returned
	assert.NotEmpty(t, response)

	// Make sure the filter exists
	ft, filterErr := fm.GetFilter(response)
	require.Nil(t, filterErr)

	assert.Equal(t, filter.BlockFilterType, ft.GetType())
}

func TestUninstallFilter_InvalidParams(t *testing.T) {
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
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			h := NewHandler(nil, nil)

			response, err := h.UninstallFilterHandler(nil, testCase.params)
			assert.Nil(t, response)

			require.NotNil(t, err)

			assert.Equal(t, spec.InvalidParamsErrorCode, err.Code)
		})
	}
}

func TestUninstallFilter_Valid(t *testing.T) {
	t.Parallel()

	t.Run("filter does not exist", func(t *testing.T) {
		t.Parallel()

		fm := filters.NewFilterManager(
			context.Background(),
			&mocks.MockStorage{},
			&mocks.MockEvents{
				SubscribeFn: func(_ []events.Type) *events.Subscription {
					return &events.Subscription{}
				},
			},
		)

		h := NewHandler(fm, nil)

		responseRaw, err := h.UninstallFilterHandler(nil, []any{"123"})
		require.Nil(t, err)

		response, ok := responseRaw.(bool)
		require.True(t, ok)

		assert.False(t, response)
	})

	t.Run("filter exists", func(t *testing.T) {
		t.Parallel()

		fm := filters.NewFilterManager(
			context.Background(),
			&mocks.MockStorage{},
			&mocks.MockEvents{
				SubscribeFn: func(_ []events.Type) *events.Subscription {
					return &events.Subscription{}
				},
			},
		)

		h := NewHandler(fm, nil)

		// Create the initial filter
		responseRaw, newFilterErr := h.NewBlockFilterHandler(nil, []any{})
		require.Nil(t, newFilterErr)

		id, ok := responseRaw.(string)
		require.True(t, ok)

		// Make sure an ID is returned
		assert.NotEmpty(t, id)

		// Uninstall the filter
		responseRaw, uninstallFilterErr := h.UninstallFilterHandler(nil, []any{id})
		require.Nil(t, uninstallFilterErr)

		response, ok := responseRaw.(bool)
		require.True(t, ok)

		assert.True(t, response)

		// Make sure the filter is actually removed
		ft, filterErr := fm.GetFilter(id)
		assert.Nil(t, ft)
		assert.Error(t, filterErr)
	})
}
