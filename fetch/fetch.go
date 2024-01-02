package fetch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
)

type Fetcher struct {
	storage Storage
	client  Client

	queryInterval time.Duration // block query interval
}

// NewFetcher creates a new transaction result fetcher instance
// that gets data from a remote chain
// TODO add logger
func NewFetcher(
	storage Storage,
	client Client,
) *Fetcher {
	return &Fetcher{
		storage:       storage,
		client:        client,
		queryInterval: 1 * time.Second,
	}
}

// FetchTransactions runs the transaction fetcher [BLOCKING]
func (f *Fetcher) FetchTransactions(ctx context.Context) error {
	// catchupWithChain syncs any transactions that have occurred
	// between the local last block (in storage) and the chain state (latest head)
	catchupWithChain := func(lastBlock int64) (int64, error) {
		// Fetch the latest block from the chain
		latest, latestErr := f.client.GetLatestBlockNumber()
		if latestErr != nil {
			return 0, fmt.Errorf("unable to fetch latest block number, %w", latestErr)
		}

		// Check if there is a block gap
		if lastBlock == latest {
			// No gap, nothing to sync
			return latest, nil
		}

		// Catch up to the latest block
		for block := lastBlock + 1; block <= latest; block++ {
			if fetchErr := f.fetchAndSaveBlockData(block); fetchErr != nil {
				return 0, fetchErr
			}
		}

		// Return the latest available block
		return latest, nil
	}

	// Fetch the latest tx from storage
	currentHeight, err := f.storage.GetLatestHeight()
	if err != nil && !errors.Is(err, storageErrors.ErrNotFound) {
		return fmt.Errorf("unable to fetch latest transaction, %w", err)
	}

	// "Catch up" initially with the chain
	if currentHeight, err = catchupWithChain(currentHeight); err != nil {
		return err
	}

	// Start a listener for monitoring new blocks
	ticker := time.NewTicker(f.queryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// TODO log
			return nil
		case <-ticker.C:
			if currentHeight, err = catchupWithChain(currentHeight); err != nil {
				return err
			}
		}
	}
}

// fetchAndSaveBlockData commits the block data to storage
func (f *Fetcher) fetchAndSaveBlockData(blockNum int64) error {
	// TODO log
	// Get block info from the chain
	block, err := f.client.GetBlock(blockNum)
	if err != nil {
		return fmt.Errorf("unable to fetch block, %w", err)
	}

	if saveErr := f.storage.SaveBlock(block.Block); saveErr != nil {
		return fmt.Errorf("unable to save block, %w", saveErr)
	}

	// Skip empty blocks
	if block.Block.NumTxs == 0 {
		return nil
	}

	// Get the transaction execution results
	txResults, err := f.client.GetBlockResults(blockNum)
	if err != nil {
		return fmt.Errorf("unable to fetch block results, %w", err)
	}

	// Save the transaction result to the storage
	for index, tx := range block.Block.Txs {
		result := &types.TxResult{
			Height:   block.Block.Height,
			Index:    uint32(index),
			Tx:       tx,
			Response: txResults.Results.DeliverTxs[index],
		}

		if err := f.storage.SaveTx(result); err != nil {
			return fmt.Errorf("unable to save tx, %w", err)
		}
	}

	return nil
}
