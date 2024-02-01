package storage

import (
	"errors"
	"fmt"

	"github.com/cockroachdb/pebble"
	"github.com/gnolang/gno/tm2/pkg/bft/types"

	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
)

var _ Storage = &Pebble{}

// Storage is the instance of an embedded storage
type Pebble struct {
	db *pebble.DB
}

// New creates a new storage instance at the given path
func NewPebble(path string) (*Pebble, error) {
	db, err := pebble.Open(path, &pebble.Options{
		// TODO: EventListener
		// Start with defaults
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create DB, %w", err)
	}

	return &Pebble{
		db: db,
	}, nil
}

// GetLatestHeight fetches the latest saved height from storage
func (s *Pebble) GetLatestHeight() (int64, error) {
	height, c, err := s.db.Get(latestHeightKey)
	if errors.Is(err, pebble.ErrNotFound) {
		return 0, storageErrors.ErrNotFound
	}

	if err != nil {
		return 0, err
	}

	defer c.Close()

	return decodeInt64(height), nil
}

// GetBlock fetches the specified block from storage, if any
func (s *Pebble) GetBlock(blockNum int64) (*types.Block, error) {
	block, c, err := s.db.Get(append(blockPrefix, encodeInt64(blockNum)...))
	if errors.Is(err, pebble.ErrNotFound) {
		return nil, storageErrors.ErrNotFound
	}

	if err != nil {
		return nil, err
	}

	defer c.Close()

	return decodeBlock(block)
}

// GetTx fetches the specified tx result from storage, if any
func (s *Pebble) GetTx(txHash []byte) (*types.TxResult, error) {
	tx, c, err := s.db.Get(append(txResultKey, txHash...))
	if errors.Is(err, pebble.ErrNotFound) {
		return nil, storageErrors.ErrNotFound
	}

	if err != nil {
		return nil, err
	}

	defer c.Close()

	return decodeTx(tx)
}

func (s *Pebble) WriteBatch() Batch {
	return &PebbleBatch{
		b: s.db.NewBatch(),
	}
}

func (s *Pebble) Close() error {
	return s.db.Close()
}

var _ Batch = &PebbleBatch{}

type PebbleBatch struct {
	b *pebble.Batch
}

func (b *PebbleBatch) SetLatestHeight(h int64) error {
	return b.b.Set(latestHeightKey, encodeInt64(h), pebble.NoSync)
}

func (b *PebbleBatch) SetBlock(block *types.Block) error {
	eb, err := encodeBlock(block)
	if err != nil {
		return err
	}

	return b.b.Set(
		append(blockPrefix, encodeInt64(block.Height)...),
		eb,
		pebble.NoSync,
	)
}

func (b *PebbleBatch) SetTx(tx *types.TxResult) error {
	encodedTx, err := encodeTx(tx)
	if err != nil {
		return err
	}

	return b.b.Set(
		append(txResultKey, tx.Tx.Hash()...),
		encodedTx,
		pebble.NoSync,
	)
}

func (b *PebbleBatch) Commit() error {
	return b.b.Commit(pebble.Sync)
}

// Rollback closes the pebble batch without persisting any data. error output is always nil.
func (b *PebbleBatch) Rollback() error {
	return b.b.Close()
}
