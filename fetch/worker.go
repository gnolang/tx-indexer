package fetch

import (
	"context"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

// workerInfo is the work context for the fetch routine
type workerInfo struct {
	resCh      chan<- *workerResponse // response channel
	chunkRange chunkRange             // data range
}

// workerResponse is the routine response
type workerResponse struct {
	error      error      // encountered error, if any
	chunk      *chunk     // the fetched chunk
	chunkRange chunkRange // the fetched chunk range
}

// handleChunk fetches the chunk from the client
func handleChunk(
	ctx context.Context,
	client Client,
	info *workerInfo,
) {
	var (
		err error

		c = &chunk{
			blocks:  make([]*types.Block, 0),
			results: make([][]*types.TxResult, 0),
		}
	)

	for blockNum := info.chunkRange.from; blockNum <= info.chunkRange.to; blockNum++ {
		// Get block info from the chain
		block, getErr := client.GetBlock(blockNum)
		if getErr != nil {
			break
		}

		results := make([]*types.TxResult, block.Block.NumTxs)

		if block.Block.NumTxs != 0 {
			// Get the transaction execution results
			txResults, resErr := client.GetBlockResults(blockNum)
			if resErr != nil {
				break
			}

			// Save the transaction result to the storage
			for index, tx := range block.Block.Txs {
				results[index] = &types.TxResult{
					Height:   block.Block.Height,
					Index:    uint32(index),
					Tx:       tx,
					Response: txResults.Results.DeliverTxs[index],
				}
			}
		}

		c.blocks = append(c.blocks, block.Block)
		c.results = append(c.results, results)
	}

	response := &workerResponse{
		error:      err,
		chunk:      c,
		chunkRange: info.chunkRange,
	}

	select {
	case <-ctx.Done():
	case info.resCh <- response:
	}
}
