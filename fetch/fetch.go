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

	// retrying holds the chunk ranges whose fetch failed, mapped to whether
	// the range is waiting to be respawned (true) or already back in flight
	// (false). A range leaves the map only once it has been fetched in full
	retrying map[chunkRange]bool

	maxSlots        int
	maxChunkSize    int64
	latestChunkSize int

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

	f.retrying = make(map[chunkRange]bool)

	return f
}

// FetchChainData starts the fetching process that indexes
// blockchain data. The genesis block is not handled here — the genesis
// package bootstraps it into the storage before any service starts.
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
		latestRemote, latestErr := f.client.GetLatestBlockNumber(ctx)
		if latestErr != nil {
			f.logger.Error("unable to fetch latest block number", zap.Error(latestErr))

			return nil
		}

		// Check if there is a block gap
		if latestRemote == latestLocal {
			// No gap, nothing to sync
			return nil
		}

		// Check if there is reset chains
		if latestRemote < latestLocal {
			if f.clearOnReset {
				if err := os.RemoveAll(f.dbPath); err != nil {
					return fmt.Errorf("unable to remove DB, %w", err)
				}

				return fmt.Errorf("reset chain: latestRemote(%d) < latestLocal(%d)", latestRemote, latestLocal)
			}

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

	// refetchFailedRanges respawns workers for the ranges whose previous fetch
	// failed. Their slots stay reserved with a nil chunk, so the write loop
	// cannot advance past them until a refetch succeeds
	refetchFailedRanges := func() {
		for gap, queued := range f.retrying {
			if !queued {
				// Already back in flight
				continue
			}

			f.retrying[gap] = false

			f.logger.Info(
				"Refetching range",
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

			// The channel is left open: workers still in flight deliver into
			// its buffer (or bail out on the cancelled context) and exit
			return nil
		case <-ticker.C:
			refetchFailedRanges()

			if err := attemptRangeFetch(); err != nil {
				return err
			}
		case response := <-collectorCh:
			if response.error != nil {
				f.logger.Error(
					"error encountered during chunk fetch, refetching range",
					zap.Uint64("from", response.chunkRange.from),
					zap.Uint64("to", response.chunkRange.to),
					zap.String("error", response.error.Error()),
				)

				// The chunk is dropped rather than saved. A partially fetched
				// chunk holds blocks whose transactions are missing, and
				// committing it advances the saved height past them, so the
				// fetcher would never revisit those blocks and the missing
				// transactions would be lost for good. Leaving the slot
				// reserved with a nil chunk blocks the write loop below until
				// the refetch succeeds
				f.retrying[response.chunkRange] = true

				continue
			}

			delete(f.retrying, response.chunkRange)

			// Find the slot index.
			// The reason for this search, is because the underlying
			// slots are shifted constantly to accommodate new ranges,
			// so by the time a slot is fetched, its original
			// position is not guaranteed
			index := sort.Search(f.chunkBuffer.Len(), func(i int) bool {
				return f.chunkBuffer.getSlot(i).chunkRange.from >= response.chunkRange.from
			})

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

				if err := f.writeSlot(item); err != nil {
					return err
				}
			}
		}
	}
}

func (f *Fetcher) writeSlot(s *slot) error {
	wb := f.storage.WriteBatch()

	// Save the fetched data
	for blockIndex, block := range s.chunk.blocks {
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
		txResults := s.chunk.results[blockIndex]

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
		zap.Uint64("from", s.chunkRange.from),
		zap.Uint64("to", s.chunkRange.to),
	)

	// Save the latest height data
	if err := wb.SetLatestHeight(s.chunkRange.to); err != nil {
		if rErr := wb.Rollback(); rErr != nil {
			return fmt.Errorf("unable to save latest height info, %w, %w", err, rErr)
		}

		return fmt.Errorf("unable to save latest height info, %w", err)
	}

	if err := wb.Commit(); err != nil {
		return fmt.Errorf("error persisting block information into storage, %w", err)
	}

	f.latestChunkSize = len(s.chunk.blocks)

	return nil
}

func (f *Fetcher) IsReady(ctx context.Context) (bool, error) {
	if f.latestChunkSize == int(f.maxChunkSize) {
		return false, fmt.Errorf("the data synchronization process is still in progress and hasn't "+
			"caught up with the current blockchain state. Chunk size: %d", f.latestChunkSize)
	}

	_, err := f.client.GetLatestBlockNumber(ctx)
	if err != nil {
		return false, fmt.Errorf("node RPC method is not reachable: %w", err)
	}

	return true, nil
}
