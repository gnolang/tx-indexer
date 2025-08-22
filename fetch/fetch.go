package fetch

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/gnolang/gno/gno.land/pkg/gnoland"
	"github.com/gnolang/gno/tm2/pkg/amino"
	bft_types "github.com/gnolang/gno/tm2/pkg/bft/types"
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

var errInvalidGenesisState = errors.New("invalid genesis state")

// Fetcher is an instance of the block indexer
// fetcher
type Fetcher struct {
	storage storage.Storage
	client  Client
	events  Events

	logger      *zap.Logger
	chunkBuffer *slots

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

	return f
}

func (f *Fetcher) fetchGenesisData() error {
	_, err := f.storage.GetLatestHeight()
	// Possible cases:
	// - err is ErrNotFound: the storage is empty, we execute the rest of the routine and fetch+write genesis data
	// - err is nil: the storage has a latest height, this means at least the genesis data has been written,
	//   or some blocks past it, we do nothing and return nil
	// - err is something else: there has been a storage error, we do nothing and return this error
	if !errors.Is(err, storageErrors.ErrNotFound) {
		return err
	}

	f.logger.Info("Fetching genesis")

	block, err := getGenesisBlock(f.client)
	if err != nil {
		return fmt.Errorf("failed to fetch genesis block: %w", err)
	}

	results, err := f.client.GetBlockResults(0)
	if err != nil {
		return fmt.Errorf("failed to fetch genesis results: %w", err)
	}

	if results.Results == nil {
		return errors.New("nil results")
	}

	txResults := make([]*bft_types.TxResult, len(block.Txs))

	for txIndex, tx := range block.Txs {
		result := &bft_types.TxResult{
			Height:   0,
			Index:    uint32(txIndex),
			Tx:       tx,
			Response: results.Results.DeliverTxs[txIndex],
		}

		txResults[txIndex] = result
	}

	s := &slot{
		chunk: &chunk{
			blocks:  []*bft_types.Block{block},
			results: [][]*bft_types.TxResult{txResults},
		},
		chunkRange: chunkRange{
			from: 0,
			to:   0,
		},
	}

	return f.writeSlot(s)
}

// FetchChainData starts the fetching process that indexes
// blockchain data
func (f *Fetcher) FetchChainData(ctx context.Context) error {
	// Attempt to fetch the genesis data
	if err := f.fetchGenesisData(); err != nil {
		// We treat this error as soft, to ease migration, since
		// some versions of gno networks don't support this.
		// In the future, we should hard fail if genesis is not fetch-able
		f.logger.Error("unable to fetch genesis data", zap.Error(err))
	}

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

func (f *Fetcher) IsReady() (bool, error) {
	if f.latestChunkSize == int(f.maxChunkSize) {
		return false, fmt.Errorf("the data synchronization process is still in progress and hasn't "+
			"caught up with the current blockchain state. Chunk size: %d", f.latestChunkSize)
	}

	_, err := f.client.GetLatestBlockNumber()
	if err != nil {
		return false, fmt.Errorf("node RPC method is not reachable: %w", err)
	}

	return true, nil
}

func getGenesisBlock(client Client) (*bft_types.Block, error) {
	gblock, err := client.GetGenesis()
	if err != nil {
		return nil, fmt.Errorf("unable to get genesis block: %w", err)
	}

	if gblock.Genesis == nil {
		return nil, errInvalidGenesisState
	}

	genesisState, ok := gblock.Genesis.AppState.(gnoland.GnoGenesisState)
	if !ok {
		return nil, fmt.Errorf("unknown genesis state kind '%T'", gblock.Genesis.AppState)
	}

	txs := make([]bft_types.Tx, len(genesisState.Txs))
	for i, tx := range genesisState.Txs {
		txs[i], err = amino.Marshal(tx.Tx)
		if err != nil {
			return nil, fmt.Errorf("unable to marshal genesis tx: %w", err)
		}
	}

	block := &bft_types.Block{
		Header: bft_types.Header{
			NumTxs:   int64(len(txs)),
			TotalTxs: int64(len(txs)),
			Time:     gblock.Genesis.GenesisTime,
			ChainID:  gblock.Genesis.ChainID,
		},
		Data: bft_types.Data{
			Txs: txs,
		},
	}

	return block, nil
}
