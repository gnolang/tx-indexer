package types

// Block is the block abstraction
type Block interface {
	// Hash returns the block hash
	Hash() []byte
}
