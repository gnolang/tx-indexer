package subscription

import (
	"testing"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/serve/encode"
	"github.com/gnolang/tx-indexer/serve/filters/mocks"
	"github.com/gnolang/tx-indexer/serve/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockSubscription_WriteResponse(t *testing.T) {
	t.Parallel()

	var (
		capturedWrite any

		mockBlock = &types.Block{
			Header: types.Header{
				Height: 10,
			},
		}
	)

	encodedResponse, err := encode.PrepareValue(mockBlock.Header)
	require.Nil(t, err)

	expectedBlockResponse := spec.NewJSONSubscribeResponse("", encodedResponse)

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
