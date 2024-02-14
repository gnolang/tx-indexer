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
	GetTxFn                func(int64, uint32) (*types.TxResult, error)
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

// GetTx fetches the tx using block height and transaction index
func (m *Storage) GetTx(blockNum int64, index uint32) (*types.TxResult, error) {
	if m.GetTxFn != nil {
		return m.GetTxFn(blockNum, index)
	}

	panic("not implemented")
}

// BlockIterator iterates over Blocks, limiting the results to be between the provided block numbers
func (m *Storage) BlockIterator(fromBlockNum, toBlockNum int64) (storage.Iterator[*types.Block], error) {
	panic("not implemented") // TODO: Implement
}

// TxIterator iterates over transactions, limiting the results to be between the provided block numbers
// and transaction indexes
func (m *Storage) TxIterator(
	fromBlockNum,
	toBlockNum int64,
	fromTxIndex,
	toTxIndex uint32,
) (storage.Iterator[*types.TxResult], error) {
	panic("not implemented") // TODO: Implement
}

// WriteBatch provides a batch intended to do a write action that
// can be cancelled or committed all at the same time
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
