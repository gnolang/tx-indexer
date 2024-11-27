package subscription

import (
	"testing"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnolang/tx-indexer/internal/mock"
	"github.com/gnolang/tx-indexer/serve/methods"
	"github.com/gnolang/tx-indexer/serve/spec"
)

func TestGasPriceSubscription_WriteResponse(t *testing.T) {
	t.Parallel()

	var (
		capturedWrite any

		mockBlock     = &types.Block{}
		mockGasPrices = []*methods.GasPrice{}
	)

	expectedGasPricesResponse := spec.NewJSONSubscribeResponse("", mockGasPrices)

	mockConn := &mock.Conn{
		WriteDataFn: func(data any) error {
			capturedWrite = data

			return nil
		},
	}

	// Create the block subscription
	gasPriceSubscription := NewGasPriceSubscription(mockConn)

	// Write the response
	require.NoError(t, gasPriceSubscription.WriteResponse("", mockBlock))

	// Make sure the captured data matches
	require.NotNil(t, capturedWrite)

	assert.Equal(t, expectedGasPricesResponse, capturedWrite)
}
