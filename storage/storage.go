package storage

import (
	"fmt"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/storage/pebble"
)

// Storage is the instance of an embedded storage
type Storage struct {
	db *pebble.Storage
}

// New creates a new storage instance at the given path
func New(path string) (*Storage, error) {
	db, err := pebble.NewDB(path)
	if err != nil {
		return nil, fmt.Errorf("unable to create DB, %w", err)
	}

	return &Storage{
		db: db,
	}, nil
}

func (s *Storage) Close() error {
	return s.db.Close()
}

// GetLatestHeight fetches the latest saved height from storage
func (s *Storage) GetLatestHeight() (int64, error) {
	height, err := s.db.Get(latestHeightKey)
	if err != nil {
		return 0, err
	}

	return decodeInt64(height), nil
}

// SaveLatestHeight saves the latest height to storage
func (s *Storage) SaveLatestHeight(height int64) error {
	return s.db.Set(latestHeightKey, encodeInt64(height))
}

// GetBlock fetches the specified block from storage, if any
func (s *Storage) GetBlock(blockNum int64) (*types.Block, error) {
	block, err := s.db.Get(append(blockPrefix, encodeInt64(blockNum)...))
	if err != nil {
		return nil, err
	}

	return decodeBlock(block)
}

// SaveBlock saves the specified block to storage
func (s *Storage) SaveBlock(block *types.Block) error {
	encodedBlock, err := encodeBlock(block)
	if err != nil {
		return err
	}

	return s.db.Set(
		append(blockPrefix, encodeInt64(block.Height)...),
		encodedBlock,
	)
}

// GetTx fetches the specified tx result from storage, if any
func (s *Storage) GetTx(txHash []byte) (*types.TxResult, error) {
	tx, err := s.db.Get(append(txResultKey, txHash...))
	if err != nil {
		return nil, err
	}

	return decodeTx(tx)
}

// SaveTx saves the specified tx result to storage
func (s *Storage) SaveTx(tx *types.TxResult) error {
	encodedTx, err := encodeTx(tx)
	if err != nil {
		return err
	}

	return s.db.Set(
		append(txResultKey, tx.Tx.Hash()...),
		encodedTx,
	)
}
