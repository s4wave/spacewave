package s4db

import (
	"encoding/binary"
	"sync"

	"github.com/pkg/errors"
)

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
		return 1 + 16
	}
	return 1 + uvarintLen(uint64(len(v.inline))) + len(v.inline)
}

// appendValue encodes v.
func appendValue(b []byte, v value) []byte {
	if v.isRef {
		b = append(b, 1)
		b = binary.LittleEndian.AppendUint64(b, v.ref.off)
		b = binary.LittleEndian.AppendUint32(b, v.ref.n)
		return binary.LittleEndian.AppendUint32(b, v.ref.crc)
	}
	b = append(b, 0)
	b = binary.AppendUvarint(b, uint64(len(v.inline)))
	return append(b, v.inline...)
}

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

// value reads an encoded value.
func (d *decoder) value() value {
	// Read the kind byte.
	if len(d.b) == 0 {
		d.fail()
		return value{}
	}
	kind := d.b[0]
	d.b = d.b[1:]

	// Read inline bytes or a reference.
	if kind == 0 {
		return value{inline: d.bytes()}
	}
	return value{isRef: true, ref: extentRef{off: d.u64(), n: d.u32(), crc: d.u32()}}
}

// fail records truncated input.
func (d *decoder) fail() {
	if d.err == nil {
		d.err = errors.New("truncated encoding")
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

// node is a decoded tree page.
type node struct {
	// leaf is set for leaf pages.
	leaf bool
	// keys holds the leaf keys, or the low key of each child of an inner
	// page. The first inner key is empty.
	keys [][]byte
	// vals holds the leaf values.
	vals []value
	// kids holds the child pages of an inner page.
	kids []uint64
}

// Page layout: kind byte, entry count, entries, zero padding, and a CRC-32C of
// the rest in the last four bytes.
const (
	// pageHeader is the kind byte and the two-byte count.
	pageHeader = 3
	// pageRoom is the space for entries.
	pageRoom = pageSize - pageHeader - 4
)

// leafEntrySize returns the encoded length of a leaf entry.
func leafEntrySize(key []byte, v value) int {
	return uvarintLen(uint64(len(key))) + len(key) + v.size()
}

// innerEntrySize returns the encoded length of an inner entry.
func innerEntrySize(key []byte) int {
	return uvarintLen(uint64(len(key))) + len(key) + 8
}

// encode writes n into a page.
func (n *node) encode() []byte {
	// Write the kind and count.
	b := make([]byte, pageHeader, pageSize)
	if n.leaf {
		b[0] = 1
	}
	binary.LittleEndian.PutUint16(b[1:], uint16(len(n.keys)))

	// Write each key with its value or child page.
	for i, k := range n.keys {
		b = binary.AppendUvarint(b, uint64(len(k)))
		b = append(b, k...)
		if n.leaf {
			b = appendValue(b, n.vals[i])
		} else {
			b = binary.LittleEndian.AppendUint64(b, n.kids[i])
		}
	}

	// Pad the page and seal it.
	b = b[:pageSize]
	binary.LittleEndian.PutUint32(b[pageSize-4:], checksum(b[:pageSize-4]))
	return b
}

// decodeNode reads a page, keeping references into b.
func decodeNode(b []byte) (*node, error) {
	// Check the page checksum.
	if checksum(b[:pageSize-4]) != binary.LittleEndian.Uint32(b[pageSize-4:]) {
		return nil, errors.New("page checksum mismatch")
	}

	// Size the node for its kind and count.
	count := int(binary.LittleEndian.Uint16(b[1:]))
	n := &node{leaf: b[0] == 1, keys: make([][]byte, count)}
	d := decoder{b: b[pageHeader : pageSize-4]}
	if n.leaf {
		n.vals = make([]value, count)
	} else {
		n.kids = make([]uint64, count)
	}

	// Read each key with its value or child page.
	for i := range count {
		n.keys[i] = d.bytes()
		if n.leaf {
			n.vals[i] = d.value()
		} else {
			n.kids[i] = d.u64()
		}
	}
	return n, d.err
}

// cache holds decoded pages. Pages are never changed once written, so
// entries never go stale; a page number is only reused after every snapshot
// that could read the old page has closed, and freeing drops the entry.
type cache struct {
	// mtx guards nodes.
	mtx sync.Mutex
	// nodes maps page numbers to decoded pages.
	nodes map[uint64]*node
	// max bounds the number of cached pages.
	max int
}

// get returns a cached page.
func (c *cache) get(page uint64) *node {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.nodes[page]
}

// put caches a page, evicting arbitrary pages when full.
func (c *cache) put(page uint64, n *node) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if len(c.nodes) >= c.max {
		drop := len(c.nodes) / 8
		for k := range c.nodes {
			if drop == 0 {
				break
			}
			delete(c.nodes, k)
			drop--
		}
	}
	c.nodes[page] = n
}

// drop removes freed pages.
func (c *cache) drop(start, n uint64) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	for p := start; p < start+n; p++ {
		delete(c.nodes, p)
	}
}
