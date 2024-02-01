package mock

import (
	"github.com/gnolang/gno/tm2/pkg/bft/types"

	"github.com/gnolang/tx-indexer/storage"
)

var _ storage.Storage = &Storage{}

type Storage struct {
	GetLatestSavedHeightFn func() (int64, error)
	GetWriteBatchFn        func() storage.Batch
	GetBlockFn             func(int64) (*types.Block, error)
	GetTxFn                func([]byte) (*types.TxResult, error)
}

func (m *Storage) GetLatestHeight() (int64, error) {
	if m.GetLatestSavedHeightFn != nil {
		return m.GetLatestSavedHeightFn()
	}

	return 0, nil
}

// GetBlock fetches the block by its number
func (m *Storage) GetBlock(blockNum int64) (*types.Block, error) {
	if m.GetBlockFn != nil {
		return m.GetBlockFn(blockNum)
	}

	panic("not implemented")
}

// GetTx fetches the tx using its hash
func (m *Storage) GetTx(tx []byte) (*types.TxResult, error) {
	if m.GetTxFn != nil {
		return m.GetTxFn(tx)
	}

	panic("not implemented")
}

// WriteBatch provides a batch intended to do a write action that
// can be cancelled or commited all at the same time
func (m *Storage) WriteBatch() storage.Batch {
	if m.GetWriteBatchFn != nil {
		return m.GetWriteBatchFn()
	}

	panic("not implemented")
}

func (m *Storage) Close() error {
	return nil
}

type WriteBatch struct {
	SetLatestHeightFn func(int64) error
	SetBlockFn        func(*types.Block) error
	SetTxFn           func(*types.TxResult) error
}

// SetLatestHeight saves the latest block height to the storage
func (mb *WriteBatch) SetLatestHeight(h int64) error {
	if mb.SetLatestHeightFn != nil {
		return mb.SetLatestHeightFn(h)
	}

	return nil
}

// SetBlock saves the block to the permanent storage
func (mb *WriteBatch) SetBlock(block *types.Block) error {
	if mb.SetBlockFn != nil {
		return mb.SetBlockFn(block)
	}

	return nil
}

// SetTx saves the transaction to the permanent storage
func (mb *WriteBatch) SetTx(tx *types.TxResult) error {
	if mb.SetTxFn != nil {
		return mb.SetTxFn(tx)
	}

	return nil
}

// Commit stores all the provided info on the storage and make
// it available for other storage readers
func (mb *WriteBatch) Commit() error {
	return nil
}

// Rollback rollbacks the operation not persisting the provided changes
func (mb *WriteBatch) Rollback() error {
	return nil
}
