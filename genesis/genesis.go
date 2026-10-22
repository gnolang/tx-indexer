// Package genesis bootstraps the chain genesis into the storage, once per
// database, before any service starts. Two consumers depend on it and neither
// owns it: the fetcher needs the genesis block stored to index from height 0,
// and the supply handler needs the genesis balances, where the vesting
// schedules live.
package genesis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gnolang/gno/gno.land/pkg/gnoland"
	"github.com/gnolang/gno/tm2/pkg/amino"
	bft_types "github.com/gnolang/gno/tm2/pkg/bft/types"
	"go.uber.org/zap"

	"github.com/gnolang/tx-indexer/storage"
	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
)

// defaultBackoff is the pause between bootstrap attempts
const defaultBackoff = 5 * time.Second

var (
	// ErrInvalidState means the node served something that is not a gno genesis
	ErrInvalidState = errors.New("invalid genesis state")

	// ErrChainIDMismatch means the storage and the node disagree about which
	// chain this is. Every stored block is suspect, not just the genesis data,
	// so it is never retried — it needs an operator, not another attempt
	ErrChainIDMismatch = errors.New("chain ID mismatch")
)

// Bootstrap loads the chain genesis into the storage, retrying until the node
// answers or ctx is done. It is a no-op once the storage carries a genesis for
// the same chain, so the genesis document is fetched exactly once per database
// and a restart costs nothing.
func Bootstrap(ctx context.Context, store Storage, client Client, opts ...Option) error {
	cfg := &config{
		logger:  zap.NewNop(),
		backoff: defaultBackoff,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	for {
		err := bootstrap(ctx, store, client, cfg)
		if err == nil {
			return nil
		}

		// Only transient failures are worth another attempt. A mismatch or a
		// genesis the indexer cannot read are facts about the deployment: the
		// answer will not change, and retrying would spin forever while
		// logging a warning every backoff.
		if errors.Is(err, ErrChainIDMismatch) || errors.Is(err, ErrInvalidState) {
			return err
		}

		cfg.logger.Warn("unable to bootstrap genesis, retrying", zap.Error(err))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(cfg.backoff):
		}
	}
}

func bootstrap(ctx context.Context, store Storage, client Client, cfg *config) error {
	// The chain ID is written last, so its presence means the whole genesis
	// landed. Checking it costs one small read — the balances stay untouched.
	storedChainID, storedErr := store.GetGenesisChainID()
	if storedErr != nil && !errors.Is(storedErr, storageErrors.ErrNotFound) {
		return storedErr
	}

	status, err := client.GetStatus(ctx)
	if err != nil {
		return fmt.Errorf("unable to fetch the chain status: %w", err)
	}

	chainID := status.NodeInfo.Network

	if storedErr == nil {
		if storedChainID != chainID {
			return fmt.Errorf(
				"%w: storage holds genesis for chain %q, node reports %q",
				ErrChainIDMismatch, storedChainID, chainID,
			)
		}

		return nil
	}

	cfg.logger.Info("Fetching genesis")

	doc, state, err := fetchState(ctx, client)
	if err != nil {
		return err
	}

	if doc.ChainID != chainID {
		return fmt.Errorf(
			"%w: genesis declares chain %q, node reports %q",
			ErrChainIDMismatch, doc.ChainID, chainID,
		)
	}

	// Only index the genesis block into an empty storage. Writing it into a
	// populated one would reset the latest height and force a full re-index.
	indexBlock, err := storageEmpty(store)
	if err != nil {
		return err
	}

	// One batch for everything: either the whole genesis is in the storage or
	// none of it is, so there is no half-bootstrapped state to reason about.
	wb := store.WriteBatch()

	if indexBlock {
		if err := indexGenesisBlock(ctx, client, wb, doc, state); err != nil {
			return rollback(wb, err)
		}
	}

	if err := wb.SetGenesisBalances(state.Balances); err != nil {
		return rollback(wb, fmt.Errorf("unable to save the genesis balances: %w", err))
	}

	if err := wb.SetGenesisChainID(chainID); err != nil {
		return rollback(wb, fmt.Errorf("unable to save the genesis chain ID: %w", err))
	}

	if err := wb.Commit(); err != nil {
		return fmt.Errorf("unable to persist the genesis into the storage: %w", err)
	}

	cfg.logger.Info(
		"Genesis bootstrapped",
		zap.String("chain-id", chainID),
		zap.Int("balances", len(state.Balances)),
		zap.Bool("block-indexed", indexBlock),
	)

	return nil
}

// storageEmpty reports whether the storage carries no chain data yet.
func storageEmpty(store Storage) (bool, error) {
	_, err := store.GetLatestHeight()

	switch {
	case err == nil:
		return false, nil
	case errors.Is(err, storageErrors.ErrNotFound):
		return true, nil
	default:
		return false, err
	}
}

// indexGenesisBlock adds the genesis block and its transactions to the batch.
//
// Unlike the fetcher's writeSlot, a block that cannot be stored fails the whole
// bootstrap rather than being logged and skipped. That tolerance exists so one
// unencodable legacy block cannot stop the fetcher; here it would leave block 0
// permanently missing, since a completed bootstrap is never retried.
func indexGenesisBlock(
	ctx context.Context,
	client Client,
	wb storage.Batch,
	doc *bft_types.GenesisDoc,
	state gnoland.GnoGenesisState,
) error {
	block, err := blockFrom(doc, state)
	if err != nil {
		return err
	}

	results, err := client.GetBlockResults(ctx, 0)
	if err != nil {
		return fmt.Errorf("unable to fetch the genesis results: %w", err)
	}

	if results.Results == nil {
		return errors.New("nil genesis results")
	}

	deliverTxs := results.Results.DeliverTxs
	if len(deliverTxs) < len(block.Txs) {
		// Only the first len(block.Txs) results are consumed, so a longer set
		// is harmless — a shorter one would read past the end of the slice.
		return fmt.Errorf(
			"%w: genesis results are short of the genesis block: %d txs, %d results",
			ErrInvalidState, len(block.Txs), len(deliverTxs),
		)
	}

	if err := wb.SetBlock(block); err != nil {
		return fmt.Errorf("unable to save the genesis block: %w", err)
	}

	for txIndex, tx := range block.Txs {
		if err := wb.SetTx(&bft_types.TxResult{
			Height:   0,
			Index:    uint32(txIndex),
			Tx:       tx,
			Response: deliverTxs[txIndex],
		}); err != nil {
			return fmt.Errorf("unable to save the genesis tx %d: %w", txIndex, err)
		}
	}

	return wb.SetLatestHeight(0)
}

// fetchState fetches the genesis document and decodes its gno app state. Cheap
// relative to blockFrom, which is why the two are separate.
func fetchState(
	ctx context.Context,
	client Client,
) (*bft_types.GenesisDoc, gnoland.GnoGenesisState, error) {
	gblock, err := client.GetGenesis(ctx)
	if err != nil {
		return nil, gnoland.GnoGenesisState{}, fmt.Errorf("unable to get the genesis: %w", err)
	}

	if gblock.Genesis == nil {
		return nil, gnoland.GnoGenesisState{}, ErrInvalidState
	}

	state, ok := gblock.Genesis.AppState.(gnoland.GnoGenesisState)
	if !ok {
		return nil, gnoland.GnoGenesisState{}, fmt.Errorf(
			"%w: unknown genesis state kind '%T'", ErrInvalidState, gblock.Genesis.AppState,
		)
	}

	return gblock.Genesis, state, nil
}

// blockFrom builds the genesis block from the decoded state. It marshals every
// genesis tx — the whole of a chain's initial packages — so it is only reached
// when the block is actually going to be indexed.
func blockFrom(
	doc *bft_types.GenesisDoc,
	state gnoland.GnoGenesisState,
) (*bft_types.Block, error) {
	var err error

	txs := make([]bft_types.Tx, len(state.Txs))
	for i, tx := range state.Txs {
		txs[i], err = amino.Marshal(tx.Tx)
		if err != nil {
			return nil, fmt.Errorf("unable to marshal the genesis tx %d: %w", i, err)
		}
	}

	return &bft_types.Block{
		Header: bft_types.Header{
			NumTxs:   int64(len(txs)),
			TotalTxs: int64(len(txs)),
			Time:     doc.GenesisTime,
			ChainID:  doc.ChainID,
		},
		Data: bft_types.Data{
			Txs: txs,
		},
	}, nil
}

func rollback(wb storage.Batch, cause error) error {
	if err := wb.Rollback(); err != nil {
		return fmt.Errorf("%w, and the rollback failed: %w", cause, err)
	}

	return cause
}
