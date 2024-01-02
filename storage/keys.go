package storage

// latestHeightKey is the quick lookup key
// for the latest height saved in the DB
var latestHeightKey = []byte("LATEST_HEIGHT")

var (
	// blockPrefix is the prefix for each block saved
	blockPrefix = []byte("BLOCK_")

	// txResultKey is the prefix for each tx result saved
	txResultKey = []byte("TX_RESULT_")
)
