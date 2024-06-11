package fetch

import (
	"context"
	"errors"
	"fmt"

	"github.com/gnolang/gno/gno.land/pkg/gnoland"
	"github.com/gnolang/gno/tm2/pkg/amino"
	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
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
	extractChunk := func() (*chunk, error) {
		errs := make([]error, 0)

		// Get block data from the node
		blocks, err := getBlocksFromBatch(info.chunkRange, client)
		errs = append(errs, err)

		results, err := getTxResultFromBatch(blocks, client)
		errs = append(errs, err)

		return &chunk{
			blocks:  blocks,
			results: results,
		}, errors.Join(errs...)
	}

	c, err := extractChunk()

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

// getBlocksFromBatch gets the blocks using batch requests.
// In case of encountering an error during fetching (remote temporarily closed, batch error...),
// the fetch is attempted again using sequential block fetches
func getBlocksFromBatch(chunkRange chunkRange, client Client) ([]*types.Block, error) {
	var (
		batch         = client.CreateBatch()
		fetchedBlocks = make([]*types.Block, 0)
	)

	batchStart := chunkRange.from
	if batchStart == 0 {
		block, err := getGenesisBlock(client)
		if err != nil {
			// Try to fetch sequentially
			return getBlocksSequentially(chunkRange, client)
		}
		batchStart += 1
		fetchedBlocks = append(fetchedBlocks, block)
	}

	// Add block requests to the batch
	for blockNum := batchStart; blockNum <= chunkRange.to; blockNum++ {
		if err := batch.AddBlockRequest(blockNum); err != nil {
			return nil, fmt.Errorf(
				"unable to add block request for block %d, %w",
				blockNum,
				err,
			)
		}
	}

	// Get the block results
	blocksRaw, err := batch.Execute(context.Background())
	if err != nil {
		// Try to fetch sequentially
		return getBlocksSequentially(chunkRange, client)
	}

	// Extract the blocks
	for _, blockRaw := range blocksRaw {
		block, ok := blockRaw.(*core_types.ResultBlock)
		if !ok {
			return nil, errors.New("unable to cast batch result into ResultBlock")
		}

		// Save block
		fetchedBlocks = append(fetchedBlocks, block.Block)
	}

	return fetchedBlocks, nil
}

// getBlocksSequentially attempts to fetch blocks from the client, using sequential requests
func getBlocksSequentially(chunkRange chunkRange, client Client) ([]*types.Block, error) {
	var (
		errs   = make([]error, 0)
		blocks = make([]*types.Block, 0)
	)

	for blockNum := chunkRange.from; blockNum <= chunkRange.to; blockNum++ {
		if blockNum == 0 {
			block, err := getGenesisBlock(client)
			if err != nil {
				errs = append(errs, fmt.Errorf("unable to get block %d, %w", blockNum, err))
				continue
			}

			blocks = append(blocks, block)
			continue
		}

		// Get block info from the chain
		block, err := client.GetBlock(blockNum)
		if err != nil {
			errs = append(errs, fmt.Errorf("unable to get block %d, %w", blockNum, err))
			continue
		}

		blocks = append(blocks, block.Block)
	}

	return blocks, errors.Join(errs...)
}

func getGenesisBlock(client Client) (*types.Block, error) {
	gblock, err := client.GetGenesis()
	if err != nil {
		return nil, fmt.Errorf("unable to get genesis block, %w", err)
	}

	genesisState, ok := gblock.Genesis.AppState.(gnoland.GnoGenesisState)
	if !ok {
		return nil, fmt.Errorf("unknown genesis state kind")
	}

	txs := make([]types.Tx, len(genesisState.Txs))
	for i, tx := range genesisState.Txs {
		txs[i], err = amino.MarshalJSON(tx)
		if err != nil {
			return nil, fmt.Errorf("unable to marshal genesis tx, %w", err)
		}
	}

	block := &types.Block{
		Header: types.Header{
			NumTxs:   int64(len(txs)),
			TotalTxs: int64(len(txs)),
			Time:     gblock.Genesis.GenesisTime,
			ChainID:  gblock.Genesis.ChainID,
		},
		Data: types.Data{
			Txs: txs,
		},
	}

	return block, nil
}

// getTxResultFromBatch gets the tx results using batch requests.
// In case of encountering an error during fetching (remote temporarily closed, batch error...),
// the fetch is attempted again using sequential tx result fetches
func getTxResultFromBatch(blocks []*types.Block, client Client) ([][]*types.TxResult, error) {
	var (
		batch          = client.CreateBatch()
		fetchedResults = make([][]*types.TxResult, len(blocks))
	)

	// Create the results request batch
	for _, block := range blocks {
		if block.NumTxs == 0 {
			// No need to request results
			// for an empty block
			continue
		}

		// Add the request to the batch
		if err := batch.AddBlockResultsRequest(uint64(block.Height)); err != nil {
			return nil, fmt.Errorf(
				"unable to add block results request for block %d, %w",
				block.Height,
				err,
			)
		}
	}

	// Check if there is anything to execute
	if batch.Count() == 0 {
		// Batch is empty, nothing to fetch
		return fetchedResults, nil
	}

	// Get the block results
	blockResultsRaw, err := batch.Execute(context.Background())
	if err != nil {
		// Try to fetch sequentially
		return getTxResultsSequentially(blocks, client)
	}

	// Extract the results
	for resultsIndex, resultsRaw := range blockResultsRaw {
		results, ok := resultsRaw.(*core_types.ResultBlockResults)
		if !ok {
			return nil, errors.New("unable to cast batch result into ResultBlockResults")
		}

		height := results.Height
		deliverTxs := results.Results.DeliverTxs

		txResults := make([]*types.TxResult, blocks[resultsIndex].NumTxs)

		for txIndex, tx := range blocks[resultsIndex].Txs {
			result := &types.TxResult{
				Height:   height,
				Index:    uint32(txIndex),
				Tx:       tx,
				Response: deliverTxs[txIndex],
			}

			txResults[txIndex] = result
		}

		fetchedResults[resultsIndex] = txResults
	}

	return fetchedResults, nil
}

// getTxResultsSequentially attempts to fetch tx results from the client, using sequential requests
func getTxResultsSequentially(blocks []*types.Block, client Client) ([][]*types.TxResult, error) {
	var (
		errs    = make([]error, 0)
		results = make([][]*types.TxResult, len(blocks))
	)

	for index, block := range blocks {
		if block.NumTxs == 0 {
			continue
		}

		// Get the transaction execution results
		blockResults, err := client.GetBlockResults(uint64(block.Height))
		if err != nil {
			errs = append(
				errs,
				fmt.Errorf(
					"unable to get block results for block %d, %w",
					block.Height,
					err,
				),
			)

			continue
		}

		// Save the transaction result
		txResults := make([]*types.TxResult, block.NumTxs)

		for index, tx := range block.Txs {
			result := &types.TxResult{
				Height:   block.Height,
				Index:    uint32(index),
				Tx:       tx,
				Response: blockResults.Results.DeliverTxs[index],
			}

			txResults[index] = result
		}

		results[index] = txResults
	}

	return results, errors.Join(errs...)
}
