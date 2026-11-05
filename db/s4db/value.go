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
	// valueDeleted marks a deleted key in a commit record or a run.
	valueDeleted = 2
)

// refSize is the encoded length of a reference without its kind byte.
const refSize = 16

// value is a stored value: inline bytes, or a reference to packed bytes. In
// a run it may be a tombstone, which hides the key from the runs and the base
// below it.
type value struct {
	// inline holds a small value.
	inline []byte
	// ref locates a large value when isRef is set.
	ref extentRef
	// isRef selects ref.
	isRef bool
	// del marks a tombstone.
	del bool
}

// size returns the encoded length of v.
func (v value) size() int {
	switch {
	case v.del:
		return 1
	case v.isRef:
		return 1 + refSize
	}
	return 1 + uvarintLen(uint64(len(v.inline))) + len(v.inline)
}

// append encodes v onto b.
func (v value) append(b []byte) []byte {
	// A tombstone is its tag alone; a ref is its tag and the extent.
	if v.del {
		return append(b, valueDeleted)
	}
	if v.isRef {
		b = append(b, valueRef)
		b = binary.LittleEndian.AppendUint64(b, v.ref.off)
		b = binary.LittleEndian.AppendUint32(b, v.ref.n)
		return binary.LittleEndian.AppendUint32(b, v.ref.crc)
	}

	// An inline value is its tag, length and bytes.
	b = append(b, valueInline)
	b = binary.AppendUvarint(b, uint64(len(v.inline)))
	return append(b, v.inline...)
}
