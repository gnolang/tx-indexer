package client

import (
	"fmt"

	rpcClient "github.com/gnolang/gno/tm2/pkg/bft/rpc/client"
)

// Batch is the wrapper for HTTP batch requests
type Batch struct {
	batch *rpcClient.BatchHTTP
}

// AddBlockRequest adds a new block request (block fetch) to the batch
func (b *Batch) AddBlockRequest(blockNum int64) error {
	if _, err := b.batch.Block(&blockNum); err != nil {
		return fmt.Errorf("unable to add block request, %w", err)
	}

	return nil
}

// AddBlockResultsRequest adds a new block results request (block results fetch) to the batch
func (b *Batch) AddBlockResultsRequest(blockNum int64) error {
	if _, err := b.batch.BlockResults(&blockNum); err != nil {
		return fmt.Errorf("unable to add block results request, %w", err)
	}

	return nil
}

// Execute sends the batch off for processing by the node
func (b *Batch) Execute() ([]any, error) {
	return b.batch.Send()
}

// Count returns the number of requests in the batch
func (b *Batch) Count() int {
	return b.batch.Count()
}
