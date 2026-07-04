package fetch

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"go.uber.org/zap"
)

// retryConfig controls how failed block / tx-result fetches are retried.
// Only the heights that actually failed are retried; already-fetched
// blocks are kept.
type retryConfig struct {
	maxRetries int           // number of retry rounds for missing heights
	retryDelay time.Duration // fixed wait between retry rounds
}

// defaultRetryConfig retries missing heights up to 5 times, waiting 2s between
// rounds. Sized to ride out short RPC blips (e.g. a 502) without hammering an
// unhealthy node. Tune via WithChunkFetchRetry.
var defaultRetryConfig = retryConfig{
	maxRetries: 5,
	retryDelay: 2 * time.Second,
}

// sleepWithContext sleeps for d, returning early if the context is cancelled.
func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// workerInfo is the work context for the fetch routine
type workerInfo struct {
	resCh      chan<- *workerResponse
	logger     *zap.Logger // may be nil
	chunkRange chunkRange
	retry      retryConfig
}

// workerResponse is the routine response
type workerResponse struct {
	error error
	chunk *chunk

	// missingBlocks are heights in the range still unavailable after retries,
	// to be scheduled for backfill rather than skipped.
	missingBlocks []uint64

	chunkRange chunkRange
}

// handleChunk fetches the chunk, retrying failed heights so transient RPC
// errors do not silently drop blocks. Heights still unavailable after all
// retries are reported in missingBlocks for later backfill.
func handleChunk(
	ctx context.Context,
	client Client,
	info *workerInfo,
) {
	logger := info.logger
	if logger == nil {
		logger = zap.NewNop()
	}

	c, missing, err := fetchChunk(ctx, client, info.chunkRange, info.retry, logger)

	response := &workerResponse{
		error:         err,
		chunk:         c,
		chunkRange:    info.chunkRange,
		missingBlocks: missing,
	}

	select {
	case <-ctx.Done():
	case info.resCh <- response:
	}
}

// fetchChunk fetches the blocks and tx results for the range, retrying only
// the heights that fail. The returned chunk holds only fully-fetched blocks
// (block + tx results); heights that could not be completed are returned in
// missing for later backfill.
func fetchChunk(
	ctx context.Context,
	client Client,
	r chunkRange,
	retry retryConfig,
	logger *zap.Logger,
) (*chunk, []uint64, error) {
	blocks := fetchBlocksWithRetry(ctx, client, r, retry, logger)

	// A block whose results can't be fetched is dropped from the chunk (and
	// reported as missing), so it is re-fetched whole later rather than
	// persisted without its transactions.
	results, incomplete := fetchResultsWithRetry(ctx, client, blocks, retry, logger)

	completeBlocks := make([]*types.Block, 0, len(blocks))
	completeResults := make([][]*types.TxResult, 0, len(blocks))

	for i, block := range blocks {
		if incomplete[i] {
			continue
		}

		completeBlocks = append(completeBlocks, block)
		completeResults = append(completeResults, results[i])
	}

	have := make(map[uint64]struct{}, len(completeBlocks))
	for _, block := range completeBlocks {
		have[uint64(block.Height)] = struct{}{}
	}

	missing := missingHeights(r, have)

	var err error
	if len(missing) > 0 {
		err = fmt.Errorf("unable to fully fetch %d block(s) in range [%d, %d]", len(missing), r.from, r.to)
	}

	return &chunk{
		blocks:  completeBlocks,
		results: completeResults,
	}, missing, err
}

// missingHeights returns the heights in the range absent from have, ascending.
func missingHeights[V any](r chunkRange, have map[uint64]V) []uint64 {
	var missing []uint64

	for h := r.from; h <= r.to; h++ {
		if _, ok := have[h]; !ok {
			missing = append(missing, h)
		}
	}

	return missing
}

// fetchBlocksWithRetry fetches all blocks in the range, retrying only the
// heights that fail and keeping the ones already fetched. The result is sorted
// by height ascending.
func fetchBlocksWithRetry(
	ctx context.Context,
	client Client,
	r chunkRange,
	retry retryConfig,
	logger *zap.Logger,
) []*types.Block {
	have := make(map[uint64]*types.Block)

	// First pass: batch fetch (falls back to sequential internally), keeping
	// whatever partial results come back.
	// The error is intentionally ignored:
	// any heights missing from the partial result are retried below.
	blocks, _ := getBlocksFromBatch(ctx, r, client) //nolint:errcheck // partial results retried below
	for _, block := range blocks {
		have[uint64(block.Height)] = block
	}

	missing := missingHeights(r, have)

	for attempt := 0; len(missing) > 0 && attempt < retry.maxRetries; attempt++ {
		if err := sleepWithContext(ctx, retry.retryDelay); err != nil {
			break // context cancelled
		}

		logger.Warn(
			"retrying missing blocks",
			zap.Int("attempt", attempt+1),
			zap.Int("count", len(missing)),
			zap.Uint64("from", r.from),
			zap.Uint64("to", r.to),
		)

		for _, height := range missing {
			block, err := client.GetBlock(ctx, height)
			if err != nil {
				continue
			}

			have[height] = block.Block
		}

		missing = missingHeights(r, have)
	}

	return sortedBlocks(have)
}

// sortedBlocks returns the map's blocks sorted by height ascending.
func sortedBlocks(have map[uint64]*types.Block) []*types.Block {
	blocks := make([]*types.Block, 0, len(have))
	for _, block := range have {
		blocks = append(blocks, block)
	}

	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Height < blocks[j].Height
	})

	return blocks
}

// fetchResultsWithRetry fetches tx results for the blocks, retrying only the
// ones that fail. It returns the results aligned to the blocks slice, plus the
// set (by block index) whose results are still missing after retries.
func fetchResultsWithRetry(
	ctx context.Context,
	client Client,
	blocks []*types.Block,
	retry retryConfig,
	logger *zap.Logger,
) ([][]*types.TxResult, map[int]bool) {
	// First pass: batch fetch (falls back to sequential internally).
	// The error is intentionally ignored:
	// any blocks whose results are missing are retried below.
	results, _ := getTxResultFromBatch(ctx, blocks, client) //nolint:errcheck // missing results retried below
	if results == nil {
		results = make([][]*types.TxResult, len(blocks))
	}

	incomplete := make(map[int]bool)

	for i, block := range blocks {
		if block.NumTxs > 0 && results[i] == nil {
			incomplete[i] = true
		}
	}

	// Retry rounds: only re-request the blocks whose results are still missing
	for attempt := 0; len(incomplete) > 0 && attempt < retry.maxRetries; attempt++ {
		if err := sleepWithContext(ctx, retry.retryDelay); err != nil {
			break
		}

		logger.Warn(
			"retrying missing tx results",
			zap.Int("attempt", attempt+1),
			zap.Int("count", len(incomplete)),
		)

		for i := range incomplete {
			block := blocks[i]

			blockResults, err := client.GetBlockResults(ctx, uint64(block.Height))
			if err != nil {
				continue
			}

			txResults, err := buildTxResults(block, blockResults)
			if err != nil {
				continue
			}

			results[i] = txResults
			delete(incomplete, i)
		}
	}

	return results, incomplete
}

// buildTxResults assembles the per-tx results for a block from the block
// results response.
func buildTxResults(block *types.Block, blockResults *core_types.ResultBlockResults) ([]*types.TxResult, error) {
	if blockResults == nil || blockResults.Results == nil {
		return nil, errors.New("nil block results")
	}

	deliverTxs := blockResults.Results.DeliverTxs
	if len(deliverTxs) < len(block.Txs) {
		return nil, fmt.Errorf(
			"block %d results count mismatch: got %d, want %d",
			block.Height,
			len(deliverTxs),
			len(block.Txs),
		)
	}

	txResults := make([]*types.TxResult, block.NumTxs)
	for txIndex, tx := range block.Txs {
		txResults[txIndex] = &types.TxResult{
			Height:   block.Height,
			Index:    uint32(txIndex),
			Tx:       tx,
			Response: deliverTxs[txIndex],
		}
	}

	return txResults, nil
}

// getBlocksFromBatch gets the blocks using batch requests.
// In case of encountering an error during fetching (remote temporarily closed, batch error...),
// the fetch is attempted again using sequential block fetches
func getBlocksFromBatch(ctx context.Context, chunkRange chunkRange, client Client) ([]*types.Block, error) {
	var (
		batch         = client.CreateBatch()
		fetchedBlocks = make([]*types.Block, 0)
	)

	// Add block requests to the batch
	for blockNum := chunkRange.from; blockNum <= chunkRange.to; blockNum++ {
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
		return getBlocksSequentially(ctx, chunkRange, client)
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
func getBlocksSequentially(ctx context.Context, chunkRange chunkRange, client Client) ([]*types.Block, error) {
	var (
		errs   = make([]error, 0)
		blocks = make([]*types.Block, 0)
	)

	for blockNum := chunkRange.from; blockNum <= chunkRange.to; blockNum++ {
		// Get block info from the chain
		block, err := client.GetBlock(ctx, blockNum)
		if err != nil {
			errs = append(errs, fmt.Errorf("unable to get block %d, %w", blockNum, err))

			continue
		}

		blocks = append(blocks, block.Block)
	}

	return blocks, errors.Join(errs...)
}

// getTxResultFromBatch gets the tx results using batch requests.
// In case of encountering an error during fetching (remote temporarily closed, batch error...),
// the fetch is attempted again using sequential tx result fetches
func getTxResultFromBatch(ctx context.Context, blocks []*types.Block, client Client) ([][]*types.TxResult, error) {
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
		return getTxResultsSequentially(ctx, blocks, client)
	}

	indexOfBlockHeight := make(map[int64]int, len(blocks))

	for index, block := range blocks {
		indexOfBlockHeight[block.Height] = index
	}

	// Extract the results
	for _, resultsRaw := range blockResultsRaw {
		results, ok := resultsRaw.(*core_types.ResultBlockResults)
		if !ok {
			return nil, errors.New("unable to cast batch result into ResultBlockResults")
		}

		height := results.Height

		blockIndex, found := indexOfBlockHeight[height]
		if !found {
			return nil, fmt.Errorf(
				"received block results for block %d, which is outside the fetched range",
				height,
			)
		}

		txResults, err := buildTxResults(blocks[blockIndex], results)
		if err != nil {
			return nil, err
		}

		// Empty blocks are left out of the results batch, so its responses are
		// dense over the blocks carrying transactions, while fetchedResults is
		// indexed over every block in the range. Pairing the two by response
		// position attributes results to the wrong blocks
		fetchedResults[blockIndex] = txResults
	}

	return fetchedResults, nil
}

// getTxResultsSequentially attempts to fetch tx results from the client, using sequential requests
func getTxResultsSequentially(ctx context.Context, blocks []*types.Block, client Client) ([][]*types.TxResult, error) {
	var (
		errs    = make([]error, 0)
		results = make([][]*types.TxResult, len(blocks))
	)

	for index, block := range blocks {
		if block.NumTxs == 0 {
			continue
		}

		// Get the transaction execution results
		blockResults, err := client.GetBlockResults(ctx, uint64(block.Height))
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
		txResults, err := buildTxResults(block, blockResults)
		if err != nil {
			errs = append(errs, err)

			continue
		}

		results[index] = txResults
	}

	return results, errors.Join(errs...)
}
