package types

// Batch defines the interface for the client batch
type Batch interface {
	// AddBlockRequest adds a new block request (block fetch) to the batch
	AddBlockRequest(int64) error

	// AddBlockResultsRequest adds a new block results request (block results fetch) to the batch
	AddBlockResultsRequest(int64) error

	// Execute sends the batch off for processing by the node
	Execute() ([]any, error)

	// Count returns the number of requests in the batch
	Count() int
}
