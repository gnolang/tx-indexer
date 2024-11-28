package storage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"unsafe"

	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/pkg/errors"
)

const (
	bytesMarker byte = 0x12

	// <term>     -> \x00\x01
	// \x00       -> \x00\xff
	escape      byte = 0x00
	escapedTerm byte = 0x01
	escaped00   byte = 0xff
	escapedFF   byte = 0x00
)

type escapes struct {
	escape      byte
	escapedTerm byte
	escaped00   byte
	escapedFF   byte
	marker      byte
}

var ascendingBytesEscapes = escapes{escape, escapedTerm, escaped00, escapedFF, bytesMarker}

// encodeUint32Ascending encodes the uint32 value using a big-endian 4 byte
// representation. The bytes are appended to the supplied buffer and
// the final buffer is returned.
func encodeUint32Ascending(b []byte, v uint32) []byte {
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// decodeUint32Ascending decodes a uint32 from the input buffer, treating
// the input as a big-endian 4 byte uint32 representation. The remainder
// of the input buffer and the decoded uint32 are returned.
//
//nolint:unparam // We want to keep all returning params
func decodeUint32Ascending(b []byte) ([]byte, uint32, error) {
	if len(b) < 4 {
		return nil, 0, fmt.Errorf("insufficient bytes to decode uint32 int value")
	}

	v := uint32(b[3]) | uint32(b[2])<<8 | uint32(b[1])<<16 | uint32(b[0])<<24

	return b[4:], v, nil
}

// encodeUint64Ascending encodes the uint64 value using a big-endian 8 byte
// representation. The bytes are appended to the supplied buffer and
// the final buffer is returned.
func encodeUint64Ascending(b []byte, v uint64) []byte {
	return append(b,
		byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32),
		byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// decodeUint64Ascending decodes a uint64 from the input buffer, treating
// the input as a big-endian 8 byte uint64 representation. The remainder
// of the input buffer and the decoded uint64 are returned.
func decodeUint64Ascending(b []byte) ([]byte, uint64, error) {
	if len(b) < 8 {
		return nil, 0, errors.Errorf("insufficient bytes to decode uint64 int value")
	}

	v := binary.BigEndian.Uint64(b)

	return b[8:], v, nil
}

// dncodeStringAscending encodes the string value using an escape-based encoding. See
// EncodeBytes for details. The encoded bytes are append to the supplied buffer
// and the resulting buffer is returned.
func encodeStringAscending(b []byte, s string) []byte {
	return encodeStringAscendingWithTerminatorAndPrefix(b, s, ascendingBytesEscapes.escapedTerm, bytesMarker)
}

// encodeStringAscendingWithTerminatorAndPrefix encodes the string value using an escape-based encoding. See
// EncodeBytes for details. The encoded bytes are append to the supplied buffer
// and the resulting buffer is returned. We can also pass a terminator byte to be used with
// JSON key encoding.
func encodeStringAscendingWithTerminatorAndPrefix(
	b []byte, s string, terminator byte, prefix byte,
) []byte {
	unsafeString := unsafeConvertStringToBytes(s)

	return encodeBytesAscendingWithTerminatorAndPrefix(b, unsafeString, terminator, prefix)
}

// encodeBytesAscendingWithTerminatorAndPrefix encodes the []byte value using an escape-based
// encoding. The encoded value is terminated with the sequence
// "\x00\terminator". The encoded bytes are append to the supplied buffer
// and the resulting buffer is returned. The terminator allows us to pass
// different terminators for things such as JSON key encoding.
func encodeBytesAscendingWithTerminatorAndPrefix(
	b []byte, data []byte, terminator byte, prefix byte,
) []byte {
	b = append(b, prefix)

	return encodeBytesAscendingWithTerminator(b, data, terminator)
}

// encodeBytesAscendingWithTerminator encodes the []byte value using an escape-based
// encoding. The encoded value is terminated with the sequence
// "\x00\terminator". The encoded bytes are append to the supplied buffer
// and the resulting buffer is returned. The terminator allows us to pass
// different terminators for things such as JSON key encoding.
func encodeBytesAscendingWithTerminator(b, data []byte, terminator byte) []byte {
	bs := encodeBytesAscendingWithoutTerminatorOrPrefix(b, data)

	return append(bs, escape, terminator)
}

// encodeBytesAscendingWithoutTerminatorOrPrefix encodes the []byte value using an escape-based
// encoding.
func encodeBytesAscendingWithoutTerminatorOrPrefix(b, data []byte) []byte {
	for {
		// IndexByte is implemented by the go runtime in assembly and is
		// much faster than looping over the bytes in the slice.
		i := bytes.IndexByte(data, escape)
		if i == -1 {
			break
		}

		b = append(b, data[:i]...)
		b = append(b, escape, escaped00)

		data = data[i+1:]
	}

	return append(b, data...)
}

// unsafeConvertStringToBytes converts a string to a byte array to be used with
// string encoding functions. Note that the output byte array should not be
// modified if the input string is expected to be used again - doing so could
// violate Go semantics.
func unsafeConvertStringToBytes(s string) []byte {
	// unsafe.StringData output is unspecified for empty string input so always
	// return nil.
	if s == "" {
		return nil
	}

	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// decodeUnsafeStringAscending decodes a string value from the input buffer which was
// encoded using EncodeString or EncodeBytes. The r []byte is used as a
// temporary buffer in order to avoid memory allocations. The remainder of the
// input buffer and the decoded string are returned. Note that the returned
// string may share storage with the input buffer.
//
//nolint:unparam // We want to keep all returning params
func decodeUnsafeStringAscending(b, r []byte) ([]byte, string, error) {
	b, r, err := decodeBytesAscending(b, r)

	return b, unsafeConvertBytesToString(r), err
}

// unsafeConvertBytesToString performs an unsafe conversion from a []byte to a
// string. The returned string will share the underlying memory with the
// []byte which thus allows the string to be mutable through the []byte. We're
// careful to use this method only in situations in which the []byte will not
// be modified.
func unsafeConvertBytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// decodeBytesAscending decodes a []byte value from the input buffer
// which was encoded using EncodeBytesAscending. The decoded bytes
// are appended to r. The remainder of the input buffer and the
// decoded []byte are returned.
func decodeBytesAscending(b, r []byte) ([]byte, []byte, error) {
	return decodeBytesInternal(b, r, ascendingBytesEscapes, true /* expectMarker */, false /* deepCopy */)
}

// decodeBytesInternal decodes an encoded []byte value from b and appends it to
// r. The remainder of b and the decoded []byte are returned. If deepCopy is
// true, then the decoded []byte will be deep copied from b and there will no
// aliasing of the same memory.
func decodeBytesInternal(
	b []byte, r []byte, e escapes, expectMarker bool, deepCopy bool,
) ([]byte, []byte, error) {
	if expectMarker {
		if len(b) == 0 || b[0] != e.marker {
			return nil, nil, fmt.Errorf("did not find marker %#x in buffer %#x", e.marker, b)
		}

		b = b[1:]
	}

	for {
		i := bytes.IndexByte(b, e.escape)
		if i == -1 {
			return nil, nil, fmt.Errorf("did not find terminator %#x in buffer %#x", e.escape, b)
		}

		if i+1 >= len(b) {
			return nil, nil, fmt.Errorf("malformed escape in buffer %#x", b)
		}

		v := b[i+1]
		if v == e.escapedTerm {
			if r == nil && !deepCopy {
				r = b[:i]
			} else {
				r = append(r, b[:i]...)
			}

			return b[i+2:], r, nil
		}

		if v != e.escaped00 {
			return nil, nil, fmt.Errorf("unknown escape sequence: %#x %#x", e.escape, v)
		}

		r = append(r, b[:i]...)
		r = append(r, e.escapedFF)
		b = b[i+2:]
	}
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
