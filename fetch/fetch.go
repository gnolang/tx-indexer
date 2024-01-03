package fetch

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"time"

	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
	queue "github.com/madz-lab/insertion-queue"
	"go.uber.org/zap"
)

const (
	maxSlots     = 100
	maxChunkSize = 100
)

type Fetcher struct {
	storage Storage
	client  Client

	logger        *zap.Logger
	chunkBuffer   *slots
	queryInterval time.Duration // block query interval
}

// New creates a new data fetcher instance
// that gets blockchain data from a remote chain
func New(
	storage Storage,
	client Client,
	opts ...Option,
) *Fetcher {
	f := &Fetcher{
		storage:       storage,
		client:        client,
		queryInterval: 1 * time.Second,
		logger:        zap.NewNop(),
		chunkBuffer:   &slots{Queue: make([]queue.Item, 0), maxSlots: maxSlots},
	}

	for _, opt := range opts {
		opt(f)
	}

	return f
}

func (f *Fetcher) FetchTransactions(ctx context.Context) error {
	defer func() {
		f.logger.Info("Fetcher service shut down")
	}()

	collectorCh := make(chan *workerResponse, maxSlots)

	startRangeFetch := func() error {
		// Check if there are any free slots
		if f.chunkBuffer.Len() == maxSlots {
			// Currently no free slot exists
			return nil
		}

		// Fetch the latest saved height
		latestLocal, err := f.storage.GetLatestHeight()
		if err != nil && !errors.Is(err, storageErrors.ErrNotFound) {
			return fmt.Errorf("unable to fetch latest block height, %w", err)
		}

		// Fetch the latest block from the chain
		latest, latestErr := f.client.GetLatestBlockNumber()
		if latestErr != nil {
			f.logger.Error("unable to fetch latest block number", zap.Error(latestErr))

			return nil
		}

		// Check if there is a block gap
		if latest <= latestLocal {
			// No gap, nothing to sync
			return nil
		}

		gaps := f.chunkBuffer.reserveChunkRanges(
			latestLocal+1,
			latest,
			maxChunkSize,
		)

		for _, gap := range gaps {
			f.logger.Info(
				"Fetching range",
				zap.Int64("from", gap.from),
				zap.Int64("to", gap.to),
			)

			// Spawn worker
			info := &workerInfo{
				chunkRange: gap,
				resCh:      collectorCh,
			}

			go handleChunk(ctx, f.client, info)
		}

		return nil
	}

	// Start a listener for monitoring new blocks
	ticker := time.NewTicker(f.queryInterval)
	defer ticker.Stop()

	// Execute the initial "catch up" with the chain
	if err := startRangeFetch(); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := startRangeFetch(); err != nil {
				return err
			}
		case response := <-collectorCh:
			// Find the slot index
			index := sort.Search(f.chunkBuffer.Len(), func(i int) bool {
				return f.chunkBuffer.getSlot(i).chunkRange.from >= response.chunkRange.from
			})

			if response.error != nil {
				f.logger.Error(
					"error encountered during chunk fetch",
					zap.String("error", response.error.Error()),
				)
			}

			// Save the chunk
			f.chunkBuffer.setChunk(index, response.chunk)

			for f.chunkBuffer.Len() > 0 {
				item := f.chunkBuffer.getSlot(0)

				isFetched := item.chunk != nil

				if !isFetched {
					break
				}

				// Pop the next chunk
				f.chunkBuffer.PopFront()

				// Save the fetched data
				for _, block := range item.chunk.blocks {
					if saveErr := f.storage.SaveBlock(block); saveErr != nil {
						// This is a design choice that really highlights the strain
						// of keeping legacy testnets running. Current TM2 testnets
						// have blocks / transactions that are no longer compatible
						// with latest "master" changes for Amino, so these blocks / txs are ignored,
						// as opposed to this error being a show-stopper for the fetcher
						f.logger.Error("unable to save block", zap.String("err", saveErr.Error()))

						continue
					}

					f.logger.Debug("Saved block data", zap.Int64("number", block.Height))
				}

				for _, txResult := range item.chunk.results {
					if err := f.storage.SaveTx(txResult); err != nil {
						f.logger.Error("unable to  save tx", zap.String("err", err.Error()))

						continue
					}

					f.logger.Debug(
						"Saved tx",
						zap.String("hash", base64.StdEncoding.EncodeToString(txResult.Tx.Hash())),
					)
				}

				f.logger.Info(
					"Saved block and tx data for range",
					zap.Int64("from", item.chunkRange.from),
					zap.Int64("to", item.chunkRange.to),
				)

				// Save the latest height data
				if err := f.storage.SaveLatestHeight(item.chunkRange.to); err != nil {
					return fmt.Errorf("unable to save latest height info, %w", err)
				}
			}
		}
	}
}
