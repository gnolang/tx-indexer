package fetch

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"time"

	queue "github.com/madz-lab/insertion-queue"
	"go.uber.org/zap"

	"github.com/gnolang/tx-indexer/storage"
	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
	"github.com/gnolang/tx-indexer/types"
)

const (
	DefaultMaxSlots     = 100
	DefaultMaxChunkSize = 100
)

// Fetcher is an instance of the block indexer
// fetcher
type Fetcher struct {
	storage storage.Storage
	client  Client
	events  Events

	logger      *zap.Logger
	chunkBuffer *slots

	maxSlots     int
	maxChunkSize int64

	queryInterval time.Duration // block query interval
}

// New creates a new data fetcher instance
// that gets blockchain data from a remote chain
func New(
	storage storage.Storage,
	client Client,
	events Events,
	opts ...Option,
) *Fetcher {
	f := &Fetcher{
		storage:       storage,
		client:        client,
		events:        events,
		queryInterval: 1 * time.Second,
		logger:        zap.NewNop(),
		maxSlots:      DefaultMaxSlots,
		maxChunkSize:  DefaultMaxChunkSize,
	}

	for _, opt := range opts {
		opt(f)
	}

	f.chunkBuffer = &slots{
		Queue:    make([]queue.Item, 0),
		maxSlots: f.maxSlots,
	}

	return f
}

// FetchChainData starts the fetching process that indexes
// blockchain data
func (f *Fetcher) FetchChainData(ctx context.Context) error {
	collectorCh := make(chan *workerResponse, DefaultMaxSlots)

	// attemptRangeFetch compares local and remote state
	// and spawns workers to fetch chunks of the chain
	attemptRangeFetch := func() error {
		// Check if there are any free slots
		if f.chunkBuffer.Len() == f.maxSlots {
			// Currently no free slot exists
			return nil
		}

		// Fetch the latest saved height
		latestLocal, err := f.storage.GetLatestHeight()
		if err != nil && !errors.Is(err, storageErrors.ErrNotFound) {
			return fmt.Errorf("unable to fetch latest block height, %w", err)
		}

		// Fetch the latest block from the chain
		latestRemote, latestErr := f.client.GetLatestBlockNumber()
		if latestErr != nil {
			f.logger.Error("unable to fetch latest block number", zap.Error(latestErr))

			return nil
		}

		// Check if there is a block gap
		if latestRemote <= latestLocal {
			// No gap, nothing to sync
			return nil
		}

		gaps := f.chunkBuffer.reserveChunkRanges(
			latestLocal+1,
			latestRemote,
			f.maxChunkSize,
		)

		for _, gap := range gaps {
			f.logger.Info(
				"Fetching range",
				zap.Uint64("from", gap.from),
				zap.Uint64("to", gap.to),
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
	if err := attemptRangeFetch(); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			f.logger.Info("Fetcher service shut down")
			close(collectorCh)

			return nil
		case <-ticker.C:
			if err := attemptRangeFetch(); err != nil {
				return err
			}
		case response := <-collectorCh:
			// Find the slot index.
			// The reason for this search, is because the underlying
			// slots are shifted constantly to accommodate new ranges,
			// so by the time a slot is fetched, its original
			// position is not guaranteed
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
				// Peek the next sequential slot
				item := f.chunkBuffer.getSlot(0)

				if item.chunk == nil {
					// Chunk not fetched yet, nothing to do
					break
				}

				// Pop the next chunk
				f.chunkBuffer.PopFront()

				wb := f.storage.WriteBatch()

				// Save the fetched data
				for blockIndex, block := range item.chunk.blocks {
					if saveErr := wb.SetBlock(block); saveErr != nil {
						// This is a design choice that really highlights the strain
						// of keeping legacy testnets running. Current TM2 testnets
						// have blocks / transactions that are no longer compatible
						// with latest "master" changes for Amino, so these blocks / txs are ignored,
						// as opposed to this error being a show-stopper for the fetcher
						f.logger.Error("unable to save block", zap.String("err", saveErr.Error()))

						continue
					}

					f.logger.Debug("Added block data to batch", zap.Int64("number", block.Height))

					// Get block results
					txResults := item.chunk.results[blockIndex]

					// Save the fetched transaction results
					for _, txResult := range txResults {
						if err := wb.SetTx(txResult); err != nil {
							f.logger.Error("unable to  save tx", zap.String("err", err.Error()))

							continue
						}

						f.logger.Debug(
							"Added tx to batch",
							zap.String("hash", base64.StdEncoding.EncodeToString(txResult.Tx.Hash())),
						)
					}

					// Alert any listeners of a new saved block
					event := &types.NewBlock{
						Block:   block,
						Results: txResults,
					}

					f.events.SignalEvent(event)
				}

				f.logger.Info(
					"Added to batch block and tx data for range",
					zap.Uint64("from", item.chunkRange.from),
					zap.Uint64("to", item.chunkRange.to),
				)

				// Save the latest height data
				if err := wb.SetLatestHeight(item.chunkRange.to); err != nil {
					if rErr := wb.Rollback(); rErr != nil {
						return fmt.Errorf("unable to save latest height info, %w, %w", err, rErr)
					}

					return fmt.Errorf("unable to save latest height info, %w", err)
				}

				if err := wb.Commit(); err != nil {
					return fmt.Errorf("error persisting block information into storage, %w", err)
				}
			}
		}
	}
}
