package storage

import (
	"io"

	"github.com/gnolang/gno/gno.land/pkg/gnoland"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

// Storage represents the permanent storage abstraction
// for reading and writing operations
type Storage interface {
	Reader
	Writer
}

// Reader defines the transaction storage interface for read methods
type Reader interface {
	io.Closer
	// GetLatestHeight returns the latest block height from the storage
	GetLatestHeight() (uint64, error)

	// GetGenesisChainID returns the chain ID of the bootstrapped genesis, or
	// ErrNotFound if the genesis has not been fully bootstrapped into this
	// storage. It is written last, so its presence marks the bootstrap done —
	// which is why the completion check never has to decode the balances
	GetGenesisChainID() (string, error)

	// GetGenesisBalances returns the stored genesis balance rows, unfolded
	GetGenesisBalances() ([]gnoland.Balance, error)

	// GetBlock fetches the block by its number
	GetBlock(uint64) (*types.Block, error)

	// GetTx fetches the tx using the block height and the transaction index
	GetTx(blockNum uint64, index uint32) (*types.TxResult, error)

	// GetTxByHash fetches the tx using the transaction hash
	GetTxByHash(txHash string) (*types.TxResult, error)

	// BlockIterator iterates over Blocks, limiting the results to be between the provided block numbers
	BlockIterator(fromBlockNum, toBlockNum uint64) (Iterator[*types.Block], error)

	// BlockReverseIterator iterates over Blocks in reverse order,
	// limiting the results to be between the provided block numbers
	BlockReverseIterator(fromBlockNum, toBlockNum uint64) (Iterator[*types.Block], error)

	// TxIterator iterates over transactions, limiting the results to be between the provided block numbers
	// and transaction indexes
	TxIterator(fromBlockNum, toBlockNum uint64, fromTxIndex, toTxIndex uint32) (Iterator[*types.TxResult], error)

	// TxReverseIterator iterates over transactions in reverse order,
	// limiting the results to be between the provided block numbers and transaction indexes
	TxReverseIterator(fromBlockNum, toBlockNum uint64, fromTxIndex, toTxIndex uint32) (Iterator[*types.TxResult], error)
}

type Iterator[T any] interface {
	io.Closer
	Next() bool
	Error() error
	Value() (T, error)
}

// Writer defines the transaction storage interface for write methods
type Writer interface {
	io.Closer
	// WriteBatch provides a batch intended to do a write action that
	// can be cancelled or committed all at the same time
	WriteBatch() Batch
}

type Batch interface {
	// SetLatestHeight saves the latest block height to the storage
	SetLatestHeight(uint64) error
	// SetGenesisBalances saves the genesis balance rows to the permanent storage
	SetGenesisBalances(balances []gnoland.Balance) error
	// SetGenesisChainID saves the genesis chain ID. It marks the bootstrap
	// complete, so it must be committed after everything else genesis writes
	SetGenesisChainID(chainID string) error
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
