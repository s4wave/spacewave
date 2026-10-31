package s4db

import (
	"encoding/binary"
)

// Value kinds in the encodings of records and run entries.
const (
	// valueInline holds the bytes in the record or entry.
	valueInline = 0
	// valueRef locates bytes in a value chunk.
	valueRef = 1
	// valueDeleted marks a deleted key: a tombstone.
	valueDeleted = 2
	// valueMoved is a reference that replaces the key's current reference
	// only while that still equals the one the cleaner read. It appears only
	// in commit records.
	valueMoved = 3
)

// refSize is the encoded length of a reference without its kind byte.
const refSize = 16

// value is a stored value: inline bytes, a reference to a value chunk, or a
// tombstone.
type value struct {
	// inline holds a small value.
	inline []byte
	// ref locates a large value when kind is valueRef or valueMoved.
	ref ref
	// kind is the value kind.
	kind byte
}

// isRef reports whether v points into a value chunk.
func (v value) isRef() bool {
	return v.kind == valueRef || v.kind == valueMoved
}

// size returns the encoded length of v including its kind byte.
func (v value) size() int {
	switch v.kind {
	case valueInline:
		return 1 + uvarintLen(uint64(len(v.inline))) + len(v.inline)
	case valueDeleted:
		return 1
	}
	return 1 + refSize
}

// append encodes v onto b.
func (v value) append(b []byte) []byte {
	// Write the kind, then the payload the kind takes.
	b = append(b, v.kind)
	switch v.kind {
	case valueInline:
		b = binary.AppendUvarint(b, uint64(len(v.inline)))
		return append(b, v.inline...)
	case valueDeleted:
		return b
	}
	b = binary.LittleEndian.AppendUint64(b, v.ref.off)
	b = binary.LittleEndian.AppendUint32(b, v.ref.n)
	return binary.LittleEndian.AppendUint32(b, v.ref.crc)
}

// decodeValue reads a value after its kind byte.
func (d *decoder) decodeValue(kind byte) value {
	switch kind {
	case valueInline:
		return value{kind: kind, inline: d.bytes()}
	case valueDeleted:
		return value{kind: kind}
	case valueRef, valueMoved:
		return value{kind: kind, ref: ref{off: d.u64(), n: d.u32(), crc: d.u32()}}
	}
	d.fail()
	return value{}
}
