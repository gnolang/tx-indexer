package storage

import (
	"errors"
	"fmt"
	"math"

	"github.com/cockroachdb/pebble"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"go.uber.org/multierr"

	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
)

const (
	// keyLatestHeight is the quick lookup key
	// for the latest height saved in the DB
	keyLatestHeight = "/meta/lh"

	// keyBlocks is the key for each block saved. They are stored by height
	prefixKeyBlocks = "/data/blocks/"

	// keyTxs is the prefix for each transaction saved.
	prefixKeyTxs = "/data/txs/"
)

func keyTx(blockNum uint64, txIndex uint32) []byte {
	var key []byte
	key = encodeStringAscending(key, prefixKeyTxs)
	key = encodeUint64Ascending(key, blockNum)
	key = encodeUint32Ascending(key, txIndex)

	return key
}

func keyBlock(blockNum uint64) []byte {
	var key []byte
	key = encodeStringAscending(key, prefixKeyBlocks)
	key = encodeUint64Ascending(key, blockNum)

	return key
}

var _ Storage = &Pebble{}

// Pebble is the instance of an embedded storage
type Pebble struct {
	db *pebble.DB
}

// NewPebble creates a new storage instance at the given path
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
func (s *Pebble) GetLatestHeight() (uint64, error) {
	height, c, err := s.db.Get([]byte(keyLatestHeight))
	if errors.Is(err, pebble.ErrNotFound) {
		return 0, storageErrors.ErrNotFound
	}

	if err != nil {
		return 0, err
	}

	defer c.Close()

	_, val, err := decodeUint64Ascending(height)

	return val, err
}

// GetBlock fetches the specified block from storage, if any
func (s *Pebble) GetBlock(blockNum uint64) (*types.Block, error) {
	block, c, err := s.db.Get(keyBlock(blockNum))
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
func (s *Pebble) GetTx(blockNum uint64, index uint32) (*types.TxResult, error) {
	tx, c, err := s.db.Get(keyTx(blockNum, index))
	if errors.Is(err, pebble.ErrNotFound) {
		return nil, storageErrors.ErrNotFound
	}

	if err != nil {
		return nil, err
	}

	defer c.Close()

	return decodeTx(tx)
}

func (s *Pebble) BlockIterator(fromBlockNum, toBlockNum uint64) (Iterator[*types.Block], error) {
	fromKey := keyBlock(fromBlockNum)

	if toBlockNum == 0 {
		toBlockNum = math.MaxInt64
	}

	toKey := keyBlock(toBlockNum)

	snap := s.db.NewSnapshot()

	it, err := snap.NewIter(&pebble.IterOptions{
		LowerBound: fromKey,
		UpperBound: toKey,
	})
	if err != nil {
		return nil, multierr.Append(snap.Close(), err)
	}

	return &PebbleBlockIter{i: it, s: snap}, nil
}

func (s *Pebble) TxIterator(
	fromBlockNum,
	toBlockNum uint64,
	fromTxIndex,
	toTxIndex uint32,
) (Iterator[*types.TxResult], error) {
	fromKey := keyTx(fromBlockNum, fromTxIndex)

	if toBlockNum == 0 {
		toBlockNum = math.MaxInt64
	}

	if toTxIndex == 0 {
		toTxIndex = math.MaxUint32
	}

	toKey := keyTx(toBlockNum, toTxIndex)

	snap := s.db.NewSnapshot()

	it, err := snap.NewIter(&pebble.IterOptions{
		LowerBound: fromKey,
		UpperBound: toKey,
	})
	if err != nil {
		return nil, multierr.Append(snap.Close(), err)
	}

	return &PebbleTxIter{i: it, s: snap, fromIndex: fromTxIndex, toIndex: toTxIndex}, nil
}

func (s *Pebble) WriteBatch() Batch {
	return &PebbleBatch{
		b: s.db.NewBatch(),
	}
}

func (s *Pebble) Close() error {
	return s.db.Close()
}

var _ Iterator[*types.Block] = &PebbleBlockIter{}

type PebbleBlockIter struct {
	i *pebble.Iterator
	s *pebble.Snapshot

	init bool
}

func (pi *PebbleBlockIter) Next() bool {
	if !pi.init {
		pi.init = true

		return pi.i.First()
	}

	return pi.i.Valid() && pi.i.Next()
}

func (pi *PebbleBlockIter) Error() error {
	return pi.i.Error()
}

func (pi *PebbleBlockIter) Value() (*types.Block, error) {
	return decodeBlock(pi.i.Value())
}

func (pi *PebbleBlockIter) Close() error {
	return multierr.Append(pi.i.Close(), pi.s.Close())
}

var _ Iterator[*types.TxResult] = &PebbleTxIter{}

type PebbleTxIter struct {
	nextError error
	i         *pebble.Iterator
	s         *pebble.Snapshot
	fromIndex uint32
	toIndex   uint32
	init      bool
}

func (pi *PebbleTxIter) Next() bool {
	for {
		if !pi.init {
			pi.init = true
			if !pi.i.First() {
				return false
			}
		}

		if !pi.i.Valid() {
			return false
		}

		if !pi.i.Next() {
			return false
		}

		var buf []byte

		key, _, err := decodeUnsafeStringAscending(pi.i.Key(), buf)
		if err != nil {
			pi.nextError = err

			return false
		}

		key, _, err = decodeUint64Ascending(key)
		if err != nil {
			pi.nextError = err

			return false
		}

		_, txIdx, err := decodeUint32Ascending(key)
		if err != nil {
			pi.nextError = err

			return false
		}

		if txIdx >= pi.fromIndex && txIdx < pi.toIndex {
			return true
		}
	}
}

func (pi *PebbleTxIter) Error() error {
	if pi.nextError != nil {
		return pi.nextError
	}

	return pi.i.Error()
}

func (pi *PebbleTxIter) Value() (*types.TxResult, error) {
	return decodeTx(pi.i.Value())
}

func (pi *PebbleTxIter) Close() error {
	return multierr.Append(pi.i.Close(), pi.s.Close())
}

var _ Batch = &PebbleBatch{}

type PebbleBatch struct {
	b *pebble.Batch
}

func (b *PebbleBatch) SetLatestHeight(h uint64) error {
	var val []byte
	val = encodeUint64Ascending(val, h)

	return b.b.Set([]byte(keyLatestHeight), val, pebble.NoSync)
}

func (b *PebbleBatch) SetBlock(block *types.Block) error {
	eb, err := encodeBlock(block)
	if err != nil {
		return err
	}

	key := keyBlock(uint64(block.Height))

	return b.b.Set(
		key,
		eb,
		pebble.NoSync,
	)
}

func (b *PebbleBatch) SetTx(tx *types.TxResult) error {
	encodedTx, err := encodeTx(tx)
	if err != nil {
		return err
	}

	key := keyTx(uint64(tx.Height), tx.Index)

	return b.b.Set(
		key,
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
