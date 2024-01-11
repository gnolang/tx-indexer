package subs

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/serve/conns"
	"github.com/gnolang/tx-indexer/serve/filters"
	"github.com/gnolang/tx-indexer/serve/filters/filter"
	"github.com/gnolang/tx-indexer/serve/filters/mocks"
	"github.com/gnolang/tx-indexer/serve/filters/subscription"
	"github.com/gnolang/tx-indexer/serve/metadata"
	"github.com/gnolang/tx-indexer/serve/spec"
	indexerTypes "github.com/gnolang/tx-indexer/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestGetFilterChanges_Valid(t *testing.T) {
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
}

func TestSubscribe_InvalidParams(t *testing.T) {
	t.Parallel()

	t.Run("invalid connection ID", func(t *testing.T) {
		t.Parallel()

		var (
			id = "invalid connection ID"

			metadata = &metadata.Metadata{
				WebSocketID: &id,
			}

			connFetcher = &mockConnectionFetcher{
				getWSConnectionFn: func(wsID string) conns.WSConnection {
					require.Equal(t, id, wsID)

					return nil // no connection
				},
			}
		)

		h := NewHandler(nil, connFetcher)

		response, err := h.SubscribeHandler(
			metadata,
			[]any{subscription.NewHeadsEvent},
		)
		assert.Nil(t, response)

		// Check the error
		require.NotNil(t, err)

		assert.Equal(t, spec.ServerErrorCode, err.Code)
		assert.Contains(t, err.Message, fmt.Sprintf("WS connection with ID %s not found", id))
	})

	t.Run("invalid event type", func(t *testing.T) {
		t.Parallel()

		var (
			id        = "connection ID"
			eventType = "random event type"

			metadata = &metadata.Metadata{
				WebSocketID: &id,
			}

			connFetcher = &mockConnectionFetcher{
				getWSConnectionFn: func(wsID string) conns.WSConnection {
					require.Equal(t, id, wsID)

					return &mocks.MockConn{} // connection found
				},
			}
		)

		h := NewHandler(nil, connFetcher)

		response, err := h.SubscribeHandler(
			metadata,
			[]any{
				eventType,
			},
		)
		assert.Nil(t, response)

		// Check the error
		require.NotNil(t, err)

		assert.Equal(t, spec.ServerErrorCode, err.Code)
		assert.Contains(t, err.Message, fmt.Sprintf("invalid event type: %s", eventType))
	})
}

func TestSubscribe_Valid(t *testing.T) {
	t.Parallel()

	var (
		wg sync.WaitGroup

		blockNum = 10
		blocks   = generateBlocks(t, blockNum)

		eventsCh = make(chan events.Event)

		connID   = "connection ID"
		metadata = &metadata.Metadata{
			WebSocketID: &connID,
		}

		mockEvents = &mocks.MockEvents{
			SubscribeFn: func(_ []events.Type) *events.Subscription {
				return &events.Subscription{
					ID:    events.SubscriptionID(1),
					SubCh: eventsCh,
				}
			},
		}

		writtenData = make([]any, 0)
		mockConn    = &mocks.MockConn{
			WriteDataFn: func(data any) error {
				defer wg.Done()
				writtenData = append(writtenData, data)

				return nil
			},
		}
		mockConnFetcher = &mockConnectionFetcher{
			getWSConnectionFn: func(id string) conns.WSConnection {
				require.Equal(t, connID, id)

				return mockConn
			},
		}
	)

	fm := filters.NewFilterManager(
		context.Background(),
		&mocks.MockStorage{},
		mockEvents,
	)

	h := NewHandler(fm, mockConnFetcher)

	// Create the initial filter
	responseRaw, newFilterErr := h.SubscribeHandler(metadata, []any{
		subscription.NewHeadsEvent,
	})
	require.Nil(t, newFilterErr)

	id, ok := responseRaw.(string)
	require.True(t, ok)

	// Simulate a few blocks
	for _, block := range blocks {
		event := &indexerTypes.NewBlock{
			Block: block,
		}

		wg.Add(1)

		select {
		case eventsCh <- event:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out")
		}
	}

	wg.Wait()

	// Make sure the correct number of messages
	// were written down
	require.Len(t, writtenData, blockNum)

	// Convert the data
	subscribeResponses := make([]*spec.BaseJSONSubscribeResponse, len(writtenData))

	for index, data := range writtenData {
		response, ok := data.(*spec.BaseJSONSubscribeResponse)
		require.True(t, ok)

		// Verify the response
		require.Equal(t, spec.SubscriptionMethod, response.Method)
		require.Equal(t, id, response.Params.Subscription)
		require.NotNil(t, response.Params.Result)

		subscribeResponses[index] = response
	}

	// Decode the changes
	subscribeHeaders := make([]types.Header, len(subscribeResponses))

	for index, subscribeResponse := range subscribeResponses {
		var header types.Header

		result, ok := subscribeResponse.Params.Result.(string)
		require.True(t, ok)

		// Decode from base64
		decodedHeader, decodeErr := base64.StdEncoding.DecodeString(result)
		require.Nil(t, decodeErr)

		// Decode from amino
		require.NoError(t, amino.Unmarshal(decodedHeader, &header))

		subscribeHeaders[index] = header
	}

	// Make sure the correct changes were caught
	for index, header := range subscribeHeaders {
		assert.Equal(t, blocks[index].Header, header)
	}
}

func TestSubscribeUnsubscribe_InvalidParams(t *testing.T) {
	t.Parallel()

	exampleString := "example"

	commonVerification := func(
		response any,
		err *spec.BaseJSONError,
		expectedErrorCode int,
	) {
		assert.Nil(t, response)

		require.NotNil(t, err)
		assert.Equal(t, expectedErrorCode, err.Code)
	}

	testTable := []struct {
		name              string
		metadata          *metadata.Metadata
		params            []any
		expectedErrorCode int
	}{
		{
			"not WS connection",
			&metadata.Metadata{WebSocketID: nil},
			[]any{},
			spec.ServerErrorCode,
		},
		{
			"invalid param length",
			&metadata.Metadata{WebSocketID: &exampleString},
			[]any{1, 2, 3},
			spec.InvalidParamsErrorCode,
		},
		{
			"invalid param type",
			&metadata.Metadata{WebSocketID: &exampleString},
			[]any{1},
			spec.InvalidParamsErrorCode,
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			h := NewHandler(nil, nil)

			response, err := h.UnsubscribeHandler(
				testCase.metadata,
				testCase.params,
			)

			commonVerification(response, err, testCase.expectedErrorCode)

			response, err = h.SubscribeHandler(
				testCase.metadata,
				testCase.params,
			)

			commonVerification(response, err, testCase.expectedErrorCode)
		})
	}
}

func TestUnsubscribe_Valid(t *testing.T) {
	t.Parallel()

	var (
		connID   = "connection ID"
		metadata = &metadata.Metadata{
			WebSocketID: &connID,
		}

		mockEvents = &mocks.MockEvents{
			SubscribeFn: func(_ []events.Type) *events.Subscription {
				return &events.Subscription{}
			},
		}

		mockConn = &mocks.MockConn{}

		mockConnFetcher = &mockConnectionFetcher{
			getWSConnectionFn: func(id string) conns.WSConnection {
				require.Equal(t, connID, id)

				return mockConn
			},
		}
	)

	fm := filters.NewFilterManager(
		context.Background(),
		&mocks.MockStorage{},
		mockEvents,
	)

	h := NewHandler(fm, mockConnFetcher)

	// Subscribe to changes
	responseRaw, newFilterErr := h.SubscribeHandler(metadata, []any{
		subscription.NewHeadsEvent,
	})
	require.Nil(t, newFilterErr)

	id, ok := responseRaw.(string)
	require.True(t, ok)

	// Unsubscribe from changes
	responseRaw, unsubscribeErr := h.UnsubscribeHandler(metadata, []any{id})
	require.Nil(t, unsubscribeErr)

	response, ok := responseRaw.(bool)
	require.True(t, ok)

	assert.True(t, response)
}
