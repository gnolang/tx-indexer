package pebble

import (
	"errors"

	"github.com/cockroachdb/pebble"
	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
)

type Storage struct {
	db *pebble.DB
}

// NewDB initializes a new pebble DB instance at the given path
func NewDB(path string) (*Storage, error) {
	db, err := pebble.Open(path, &pebble.Options{})
	if err != nil {
		return nil, err
	}

	return &Storage{
		db: db,
	}, nil
}

// Set stores a value for the given key
func (p *Storage) Set(key, value []byte) error {
	return p.db.Set(key, value, pebble.Sync)
}

// Get retrieves the value for a given key
func (p *Storage) Get(key []byte) ([]byte, error) {
	value, closer, err := p.db.Get(key)
	if errors.Is(err, pebble.ErrNotFound) {
		// Wrap the error
		return nil, storageErrors.ErrNotFound
	}

	if err != nil {
		return nil, err
	}

	defer closer.Close()

	return value, nil
}

// Close closes the database connection
func (p *Storage) Close() error {
	return p.db.Close()
}
