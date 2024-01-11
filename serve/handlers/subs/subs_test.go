package subs

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/serve/filters"
	"github.com/gnolang/tx-indexer/serve/filters/filter"
	"github.com/gnolang/tx-indexer/serve/filters/mocks"
	"github.com/gnolang/tx-indexer/serve/spec"
	indexerTypes "github.com/gnolang/tx-indexer/types"
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

func TestGetFilterChanges_InvalidParams(t *testing.T) {
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

			response, err := h.GetFilterChangesHandler(nil, testCase.params)
			assert.Nil(t, response)

			require.NotNil(t, err)

			assert.Equal(t, spec.InvalidParamsErrorCode, err.Code)
		})
	}
}

func TestGetFilterChanges(t *testing.T) {
	t.Parallel()

	var (
		blockNum = 10
		blocks   = generateBlocks(t, blockNum)

		eventsCh = make(chan events.Event)

		mockEvents = &mocks.MockEvents{
			SubscribeFn: func(_ []events.Type) *events.Subscription {
				return &events.Subscription{
					ID:    events.SubscriptionID(1),
					SubCh: eventsCh,
				}
			},
		}
	)

	fm := filters.NewFilterManager(
		context.Background(),
		&mocks.MockStorage{},
		mockEvents,
	)

	h := NewHandler(fm, nil)

	// Create the initial filter
	responseRaw, newFilterErr := h.NewBlockFilterHandler(nil, []any{})
	require.Nil(t, newFilterErr)

	id, ok := responseRaw.(string)
	require.True(t, ok)

	// Simulate a few blocks
	for _, block := range blocks {
		event := &indexerTypes.NewBlock{
			Block: block,
		}

		select {
		case eventsCh <- event:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out")
		}
	}

	// Get filter changes
	changesRaw, changesErr := h.GetFilterChangesHandler(nil, []any{id})
	require.Nil(t, changesErr)

	// Check the changes
	require.NotNil(t, changesRaw)

	encodedChanges, ok := changesRaw.([]string)
	require.True(t, ok)

	// Decode the changes
	changes := make([]types.Header, len(encodedChanges))

	for index, encodedChange := range encodedChanges {
		var header types.Header

		// Decode from base64
		decodedHeader, decodeErr := base64.StdEncoding.DecodeString(encodedChange)
		require.Nil(t, decodeErr)

		// Decode from amino
		require.NoError(t, amino.Unmarshal(decodedHeader, &header))

		changes[index] = header
	}

	// Make sure the correct changes were caught
	for index, header := range changes {
		assert.Equal(t, blocks[index].Header, header)
	}

	// Uninstall the filter
	responseRaw, uninstallFilterErr := h.UninstallFilterHandler(nil, []any{id})
	require.Nil(t, uninstallFilterErr)

	response, ok := responseRaw.(bool)
	require.True(t, ok)

	assert.True(t, response)
}

// generateBlocks generates dummy blocks
func generateBlocks(t *testing.T, count int) []*types.Block {
	t.Helper()

	blocks := make([]*types.Block, count)

	for i := 0; i < count; i++ {
		blocks[i] = &types.Block{
			Header: types.Header{
				Height: int64(i),
			},
			Data: types.Data{},
		}
	}

	return blocks
}
