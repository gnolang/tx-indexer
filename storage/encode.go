package storage

import (
	"encoding/binary"
	"fmt"

	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

// encodeInt64 encodes an int64 value into little endian
func encodeInt64(value int64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(value))

	return b
}

// decodeInt64 decodes an int64 value from little endian
func decodeInt64(value []byte) int64 {
	return int64(binary.LittleEndian.Uint64(value))
}

// encodeBlock encodes the block in Amino binary
func encodeBlock(block *types.Block) ([]byte, error) {
	return amino.Marshal(block)
}

// decodeBlock decodes the Amino encoded block
func decodeBlock(encodedBlock []byte) (*types.Block, error) {
	var block types.Block

	if err := amino.Unmarshal(encodedBlock, &block); err != nil {
		return nil, fmt.Errorf("unable to unmarshal Amino block, %w", err)
	}

	return &block, nil
}

// encodeTx encodes the tx result in Amino binary
func encodeTx(tx *types.TxResult) ([]byte, error) {
	return amino.Marshal(tx)
}

// decodeTx decodes the Amino encoded tx result
func decodeTx(encodedTx []byte) (*types.TxResult, error) {
	var tx types.TxResult

	if err := amino.Unmarshal(encodedTx, &tx); err != nil {
		return nil, fmt.Errorf("unable to unmarshal Amino tx, %w", err)
	}

	return &tx, nil
}
