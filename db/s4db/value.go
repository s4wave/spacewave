package s4db

import (
	"encoding/binary"
)

// Value kinds in the encoding.
const (
	// valueInline holds the bytes in the index.
	valueInline = 0
	// valueRef locates packed bytes.
	valueRef = 1
	// valueDeleted marks a deleted key in a commit record.
	valueDeleted = 2
)

// refSize is the encoded length of a reference without its kind byte.
const refSize = 16

// value is a stored value: inline bytes, or a reference to packed bytes.
type value struct {
	// inline holds a small value.
	inline []byte
	// ref locates a large value when isRef is set.
	ref extentRef
	// isRef selects ref.
	isRef bool
}

// size returns the encoded length of v.
func (v value) size() int {
	if v.isRef {
		return 1 + refSize
	}
	return 1 + uvarintLen(uint64(len(v.inline))) + len(v.inline)
}

// append encodes v onto b.
func (v value) append(b []byte) []byte {
	if v.isRef {
		b = append(b, valueRef)
		b = binary.LittleEndian.AppendUint64(b, v.ref.off)
		b = binary.LittleEndian.AppendUint32(b, v.ref.n)
		return binary.LittleEndian.AppendUint32(b, v.ref.crc)
	}
	b = append(b, valueInline)
	b = binary.AppendUvarint(b, uint64(len(v.inline)))
	return append(b, v.inline...)
}
