package s4db

import (
	"encoding/binary"

	"github.com/pkg/errors"
)

// errTruncated is recorded when an encoding ends early.
var errTruncated = errors.New("truncated encoding")

// decoder reads the fields of a page or record.
type decoder struct {
	// b is the unread input.
	b []byte
	// err is set once input runs short.
	err error
}

// uvarint reads an unsigned varint.
func (d *decoder) uvarint() uint64 {
	v, n := binary.Uvarint(d.b)
	if n <= 0 {
		d.fail()
		return 0
	}
	d.b = d.b[n:]
	return v
}

// varint reads a signed varint.
func (d *decoder) varint() int64 {
	v, n := binary.Varint(d.b)
	if n <= 0 {
		d.fail()
		return 0
	}
	d.b = d.b[n:]
	return v
}

// u64 reads a little-endian uint64.
func (d *decoder) u64() uint64 {
	if len(d.b) < 8 {
		d.fail()
		return 0
	}
	v := binary.LittleEndian.Uint64(d.b)
	d.b = d.b[8:]
	return v
}

// u32 reads a little-endian uint32.
func (d *decoder) u32() uint32 {
	if len(d.b) < 4 {
		d.fail()
		return 0
	}
	v := binary.LittleEndian.Uint32(d.b)
	d.b = d.b[4:]
	return v
}

// count reads an element count, capped by the unread input so corrupt input
// cannot force a large allocation or loop: every element takes a byte.
func (d *decoder) count() uint64 {
	return min(d.uvarint(), uint64(len(d.b)))
}

// bytes reads a length-prefixed byte string without copying.
func (d *decoder) bytes() []byte {
	// Read the length and check the input holds it.
	n := d.uvarint()
	if uint64(len(d.b)) < n {
		d.fail()
		return nil
	}

	// Slice the bytes off the input.
	v := d.b[:n:n]
	d.b = d.b[n:]
	return v
}

// kind reads a value kind byte.
func (d *decoder) kind() byte {
	if len(d.b) == 0 {
		d.fail()
		return 0
	}
	k := d.b[0]
	d.b = d.b[1:]
	return k
}

// value reads the encoding of a value after its kind byte.
func (d *decoder) value(kind byte) value {
	switch kind {
	case valueInline:
		return value{inline: d.bytes()}
	case valueRef:
		return value{isRef: true, ref: extentRef{off: d.u64(), n: d.u32(), crc: d.u32()}}
	}
	d.fail()
	return value{}
}

// fail records truncated input.
func (d *decoder) fail() {
	if d.err == nil {
		d.err = errTruncated
	}
	d.b = nil
}

// uvarintLen returns the encoded length of v.
func uvarintLen(v uint64) int {
	n := 1
	for v >= 0x80 {
		v >>= 7
		n++
	}
	return n
}
