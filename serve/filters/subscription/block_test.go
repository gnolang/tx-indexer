package subscription

import (
	"testing"

	"github.com/gnolang/tx-indexer/serve/handlers/subs/filters/mocks"
	"github.com/gnolang/tx-indexer/serve/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockSubscription_WriteResponse(t *testing.T) {
	t.Parallel()

	var (
		capturedWrite any

		mockBlock = &mocks.MockBlock{
			HashFn: func() []byte {
				return []byte("hash")
			},
		}
	)

	expectedBlockResponse := spec.NewJSONSubscribeResponse("", mockBlock)

	mockConn := &mocks.MockConn{
		WriteDataFn: func(data any) error {
			capturedWrite = data

			return nil
		},
	}

	// Create the block subscription
	blockSubscription := NewBlockSubscription(mockConn)

	// Write the response
	require.NoError(t, blockSubscription.WriteResponse("", mockBlock))

	// Make sure the captured data matches
	require.NotNil(t, capturedWrite)

	assert.Equal(t, expectedBlockResponse, capturedWrite)
}
