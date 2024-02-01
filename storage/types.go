package storage

import (
	"io"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

// Storage represents the permanent storage abstraction
// for reading and writing operations
type Storage interface {
	io.Closer
	Reader
	Writer
}

// StorageRead defines the transaction storage interface for read methods
type Reader interface {
	io.Closer
	// GetLatestHeight returns the latest block height from the storage
	GetLatestHeight() (int64, error)

	// GetBlock fetches the block by its number
	GetBlock(int64) (*types.Block, error)

	// GetTx fetches the tx using its hash
	GetTx([]byte) (*types.TxResult, error)
}

// StorageWrite defines the transaction storage interface for write methods
type Writer interface {
	io.Closer
	// WriteBatch provides a batch intended to do a write action that
	// can be cancelled or committed all at the same time
	WriteBatch() Batch
}

type Batch interface {
	// SetLatestHeight saves the latest block height to the storage
	SetLatestHeight(int64) error
	// SetBlock saves the block to the permanent storage
	SetBlock(block *types.Block) error
	// SetTx saves the transaction to the permanent storage
	SetTx(tx *types.TxResult) error

	// Commit stores all the provided info on the storage and make
	// it available for other storage readers
	Commit() error

	// Rollback rollbacks the operation not persisting the provided changes
	Rollback() error
}
