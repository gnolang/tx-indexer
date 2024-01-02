package pebble

import (
	"crypto/rand"
	"testing"

	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateRandomBytes generates random bytes
func generateRandomBytes(t *testing.T, length int) []byte {
	t.Helper()

	bytes := make([]byte, length)
	_, err := rand.Read(bytes)
	require.NoError(t, err)

	return bytes
}

// generateRandomPairs generates random key value pairs
func generateRandomPairs(t *testing.T, count int) map[string][]byte {
	t.Helper()

	pairs := make(map[string][]byte, count)

	for i := 0; i < count; i++ {
		key := generateRandomBytes(t, 8)
		value := generateRandomBytes(t, 32)

		pairs[string(key)] = value
	}

	return pairs
}

func TestPebble_GetMissingItem(t *testing.T) {
	t.Parallel()

	// Initialize the pebble DB
	store, err := NewDB(t.TempDir())
	require.NoError(t, err)

	defer store.Close()

	// Fetch a non-existent value
	_, err = store.Get([]byte("non_existent_key"))
	if !assert.ErrorIs(t, err, storageErrors.ErrNotFound) {
		t.Errorf("Expected error not found when getting non-existent key")
	}
}

func TestPebble_WriteRead(t *testing.T) {
	t.Parallel()

	// Initialize the pebble DB
	store, err := NewDB(t.TempDir())
	require.NoError(t, err)

	defer store.Close()

	pairs := generateRandomPairs(t, 50)

	for key, value := range pairs {
		// Set the key
		require.NoError(t, store.Set([]byte(key), value))

		// Get the value
		retrievedValue, err := store.Get([]byte(key))
		require.NoError(t, err)

		assert.Equal(t, value, retrievedValue)
	}
}
