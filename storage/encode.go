package storage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"unsafe"

	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

const (
	bytesMarker     byte = 0x12
	bytesDescMarker      = bytesMarker + 1

	// IntMin is chosen such that the range of int tags does not overlap the
	// ascii character set that is frequently used in testing.
	IntMin      = 0x80 // 128
	intMaxWidth = 8
	intZero     = IntMin + intMaxWidth           // 136
	intSmall    = IntMax - intZero - intMaxWidth // 109
	// IntMax is the maximum int tag value.
	IntMax = 0xfd // 253

	// <term>     -> \x00\x01
	// \x00       -> \x00\xff
	escape                   byte = 0x00
	escapedTerm              byte = 0x01
	escapedJSONObjectKeyTerm byte = 0x02
	escapedJSONArray         byte = 0x03
	escaped00                byte = 0xff
	escapedFF                byte = 0x00
)

type escapes struct {
	escape      byte
	escapedTerm byte
	escaped00   byte
	escapedFF   byte
	marker      byte
}

var ascendingBytesEscapes = escapes{escape, escapedTerm, escaped00, escapedFF, bytesMarker}

// EncodeUint32Ascending encodes the uint32 value using a big-endian 4 byte
// representation. The bytes are appended to the supplied buffer and
// the final buffer is returned.
func EncodeUint32Ascending(b []byte, v uint32) []byte {
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// EncodeUint32Descending encodes the uint32 value so that it sorts in
// reverse order, from largest to smallest.
func EncodeUint32Descending(b []byte, v uint32) []byte {
	return EncodeUint32Ascending(b, ^v)
}

// DecodeUint32Ascending decodes a uint32 from the input buffer, treating
// the input as a big-endian 4 byte uint32 representation. The remainder
// of the input buffer and the decoded uint32 are returned.
func DecodeUint32Ascending(b []byte) ([]byte, uint32, error) {
	if len(b) < 4 {
		return nil, 0, fmt.Errorf("insufficient bytes to decode uint32 int value")
	}

	v := binary.BigEndian.Uint32(b)

	return b[4:], v, nil
}

// DecodeUint32Descending decodes a uint32 value which was encoded
// using EncodeUint32Descending.
func DecodeUint32Descending(b []byte) ([]byte, uint32, error) {
	leftover, v, err := DecodeUint32Ascending(b)

	return leftover, ^v, err
}

// EncodeVarintAscending encodes the int64 value using a variable length
// (length-prefixed) representation. The length is encoded as a single
// byte. If the value to be encoded is negative the length is encoded
// as 8-numBytes. If the value is positive it is encoded as
// 8+numBytes. The encoded bytes are appended to the supplied buffer
// and the final buffer is returned.
func EncodeVarintAscending(b []byte, v int64) []byte {
	if v < 0 {
		switch {
		case v >= -0xff:
			return append(b, IntMin+7, byte(v))
		case v >= -0xffff:
			return append(b, IntMin+6, byte(v>>8), byte(v))
		case v >= -0xffffff:
			return append(b, IntMin+5, byte(v>>16), byte(v>>8), byte(v))
		case v >= -0xffffffff:
			return append(b, IntMin+4, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
		case v >= -0xffffffffff:
			return append(b, IntMin+3, byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8),
				byte(v))
		case v >= -0xffffffffffff:
			return append(b, IntMin+2, byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16),
				byte(v>>8), byte(v))
		case v >= -0xffffffffffffff:
			return append(b, IntMin+1, byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24),
				byte(v>>16), byte(v>>8), byte(v))
		default:
			return append(b, IntMin, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32),
				byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
		}
	}

	return EncodeUvarintAscending(b, uint64(v))
}

// EncodeVarintDescending encodes the int64 value so that it sorts in reverse
// order, from largest to smallest.
func EncodeVarintDescending(b []byte, v int64) []byte {
	return EncodeVarintAscending(b, ^v)
}

// EncodeUvarintAscending encodes the uint64 value using a variable length
// (length-prefixed) representation. The length is encoded as a single
// byte indicating the number of encoded bytes (-8) to follow. See
// EncodeVarintAscending for rationale. The encoded bytes are appended to the
// supplied buffer and the final buffer is returned.
func EncodeUvarintAscending(b []byte, v uint64) []byte {
	switch {
	case v <= intSmall:
		return append(b, intZero+byte(v))
	case v <= 0xff:
		return append(b, IntMax-7, byte(v))
	case v <= 0xffff:
		return append(b, IntMax-6, byte(v>>8), byte(v))
	case v <= 0xffffff:
		return append(b, IntMax-5, byte(v>>16), byte(v>>8), byte(v))
	case v <= 0xffffffff:
		return append(b, IntMax-4, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	case v <= 0xffffffffff:
		return append(b, IntMax-3, byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8),
			byte(v))
	case v <= 0xffffffffffff:
		return append(b, IntMax-2, byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16),
			byte(v>>8), byte(v))
	case v <= 0xffffffffffffff:
		return append(b, IntMax-1, byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24),
			byte(v>>16), byte(v>>8), byte(v))
	default:
		return append(b, IntMax, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32),
			byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}
}

// DecodeVarintAscending decodes a value encoded by EncodeVarintAscending.
func DecodeVarintAscending(b []byte) ([]byte, int64, error) {
	if len(b) == 0 {
		return nil, 0, fmt.Errorf("insufficient bytes to decode varint value")
	}

	length := int(b[0]) - intZero
	if length < 0 {
		length = -length

		remB := b[1:]
		if len(remB) < length {
			return nil, 0, fmt.Errorf("insufficient bytes to decode varint value: %q", remB)
		}

		var v int64
		// Use the ones-complement of each encoded byte in order to build
		// up a positive number, then take the ones-complement again to
		// arrive at our negative value.
		for _, t := range remB[:length] {
			v = (v << 8) | int64(^t)
		}

		return remB[length:], ^v, nil
	}

	remB, v, err := DecodeUvarintAscending(b)
	if err != nil {
		return remB, 0, err
	}

	if v > math.MaxInt64 {
		return nil, 0, fmt.Errorf("varint %d overflows int64", v)
	}

	return remB, int64(v), nil
}

// DecodeUvarintAscending decodes a uint64 encoded uint64 from the input
// buffer. The remainder of the input buffer and the decoded uint64
// are returned.
func DecodeUvarintAscending(b []byte) ([]byte, uint64, error) {
	if len(b) == 0 {
		return nil, 0, fmt.Errorf("insufficient bytes to decode uvarint value")
	}

	length := int(b[0]) - intZero

	b = b[1:] // skip length byte
	if length <= intSmall {
		return b, uint64(length), nil
	}

	length -= intSmall
	if length < 0 || length > 8 {
		return nil, 0, fmt.Errorf("invalid uvarint length of %d", length)
	} else if len(b) < length {
		return nil, 0, fmt.Errorf("insufficient bytes to decode uvarint value: %q", b)
	}

	var v uint64
	// It is faster to range over the elements in a slice than to index
	// into the slice on each loop iteration.
	for _, t := range b[:length] {
		v = (v << 8) | uint64(t)
	}

	return b[length:], v, nil
}

// DecodeVarintDescending decodes a int64 value which was encoded
// using EncodeVarintDescending.
func DecodeVarintDescending(b []byte) ([]byte, int64, error) {
	leftover, v, err := DecodeVarintAscending(b)

	return leftover, ^v, err
}

// EncodeStringAscending encodes the string value using an escape-based encoding. See
// EncodeBytes for details. The encoded bytes are append to the supplied buffer
// and the resulting buffer is returned.
func EncodeStringAscending(b []byte, s string) []byte {
	return encodeStringAscendingWithTerminatorAndPrefix(b, s, ascendingBytesEscapes.escapedTerm, bytesMarker)
}

// encodeStringAscendingWithTerminatorAndPrefix encodes the string value using an escape-based encoding. See
// EncodeBytes for details. The encoded bytes are append to the supplied buffer
// and the resulting buffer is returned. We can also pass a terminator byte to be used with
// JSON key encoding.
func encodeStringAscendingWithTerminatorAndPrefix(
	b []byte, s string, terminator byte, prefix byte,
) []byte {
	unsafeString := UnsafeConvertStringToBytes(s)

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

// UnsafeConvertStringToBytes converts a string to a byte array to be used with
// string encoding functions. Note that the output byte array should not be
// modified if the input string is expected to be used again - doing so could
// violate Go semantics.
func UnsafeConvertStringToBytes(s string) []byte {
	// unsafe.StringData output is unspecified for empty string input so always
	// return nil.
	if s == "" {
		return nil
	}

	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// DecodeUnsafeStringAscending decodes a string value from the input buffer which was
// encoded using EncodeString or EncodeBytes. The r []byte is used as a
// temporary buffer in order to avoid memory allocations. The remainder of the
// input buffer and the decoded string are returned. Note that the returned
// string may share storage with the input buffer.
func DecodeUnsafeStringAscending(b, r []byte) ([]byte, string, error) {
	b, r, err := DecodeBytesAscending(b, r)

	return b, UnsafeConvertBytesToString(r), err
}

// UnsafeConvertBytesToString performs an unsafe conversion from a []byte to a
// string. The returned string will share the underlying memory with the
// []byte which thus allows the string to be mutable through the []byte. We're
// careful to use this method only in situations in which the []byte will not
// be modified.
func UnsafeConvertBytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// DecodeBytesAscending decodes a []byte value from the input buffer
// which was encoded using EncodeBytesAscending. The decoded bytes
// are appended to r. The remainder of the input buffer and the
// decoded []byte are returned.
func DecodeBytesAscending(b, r []byte) ([]byte, []byte, error) {
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
