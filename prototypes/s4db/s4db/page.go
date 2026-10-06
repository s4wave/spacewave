package s4db

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"sync"
	"sync/atomic"

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
	// heads holds the head of each key, so a search compares within one
	// array and reads a key only on a tie.
	heads []uint64
	// vals holds the leaf values.
	vals []value
	// kids holds the child pages of an inner page.
	kids []uint64
	// inner holds the decoded children of an inner page that are themselves
	// inner pages, filled as reads descend. A child page is freed only with
	// every page that references it, so a link never outlives its target.
	inner []atomic.Pointer[node]
}

// Page layout: kind byte, entry count, entries, zero padding, and a CRC-32C of
// the rest in the last four bytes.
const (
	// pageHeader is the kind byte and the two-byte count.
	pageHeader = 3
	// pageRoom is the space for entries.
	pageRoom = pageSize - pageHeader - 4
)

// head returns the first eight bytes of key as a big-endian integer, zero
// padded, so integers order as their keys do except on ties.
func head(key []byte) uint64 {
	if len(key) >= 8 {
		return binary.BigEndian.Uint64(key)
	}
	var b [8]byte
	copy(b[:], key)
	return binary.BigEndian.Uint64(b[:])
}

// search returns the position of the first key at or after key and whether
// it equals key.
func (n *node) search(key []byte) (int, bool) {
	// Bisect on heads, comparing whole keys on equal heads.
	h := head(key)
	lo, hi := 0, len(n.keys)
	for lo < hi {
		m := int(uint(lo+hi) >> 1)
		c := cmp.Compare(n.heads[m], h)
		if c == 0 {
			c = bytes.Compare(n.keys[m], key)
		}
		if c < 0 {
			lo = m + 1
		} else {
			hi = m
		}
	}
	return lo, lo < len(n.keys) && bytes.Equal(n.keys[lo], key)
}

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
		n.inner = make([]atomic.Pointer[node], count)
	}

	// Read each key with its value or child page.
	n.heads = make([]uint64, count)
	for i := range count {
		n.keys[i] = d.bytes()
		n.heads[i] = head(n.keys[i])
		if n.leaf {
			n.vals[i] = d.value()
		} else {
			n.kids[i] = d.u64()
		}
	}
	return n, d.err
}

// cacheShards is the number of independently locked cache shards.
const cacheShards = 64

// cache holds decoded pages in shards, each evicting by CLOCK. Pages are
// never changed once written, so entries never go stale; a page number is
// only reused after every snapshot that could read the old page has closed,
// and freeing drops the entry.
type cache struct {
	// root holds the last root page read, so lookups start without taking
	// a shard lock.
	root atomic.Pointer[cacheSlot]
	// shards hold the pages by page number.
	shards [cacheShards]cacheShard
}

// cacheShard is one locked part of the cache.
type cacheShard struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// index maps page numbers to positions in ring.
	index map[uint64]int
	// ring holds the cached pages in clock order.
	ring []cacheSlot
	// hand is the next position the clock examines.
	hand int
	// max bounds len(ring).
	max int
}

// cacheSlot is one cached page.
type cacheSlot struct {
	// page is the page number.
	page uint64
	// n is the decoded page; nil marks a free slot.
	n *node
	// used is set by a hit and cleared as the clock passes.
	used bool
}

// newCache returns a cache bounded to about pages pages.
func newCache(pages int) *cache {
	c := &cache{}
	for i := range c.shards {
		c.shards[i] = cacheShard{index: make(map[uint64]int), max: max(1, pages/cacheShards)}
	}
	return c
}

// shard returns the shard holding page.
func (c *cache) shard(page uint64) *cacheShard {
	return &c.shards[(page*0x9e3779b97f4a7c15)>>58]
}

// get returns a cached page.
func (c *cache) get(page uint64) *node {
	// Find the page in its shard.
	s := c.shard(page)
	s.mtx.Lock()
	defer s.mtx.Unlock()
	i, ok := s.index[page]
	if !ok {
		return nil
	}

	// Mark it used so the clock passes it once.
	s.ring[i].used = true
	return s.ring[i].n
}

// put caches a page. A full shard evicts the first page the clock finds
// unused since it last passed, so pages read once leave before pages read
// again.
func (c *cache) put(page uint64, n *node) {
	// Replace a cached page in place.
	s := c.shard(page)
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if i, ok := s.index[page]; ok {
		s.ring[i].n = n
		return
	}

	// Grow the ring until it is full.
	if len(s.ring) < s.max {
		s.index[page] = len(s.ring)
		s.ring = append(s.ring, cacheSlot{page: page, n: n})
		return
	}

	// Advance the clock to a free or unused slot and take it.
	for s.ring[s.hand].n != nil && s.ring[s.hand].used {
		s.ring[s.hand].used = false
		s.hand = (s.hand + 1) % len(s.ring)
	}
	if old := s.ring[s.hand]; old.n != nil {
		delete(s.index, old.page)
	}
	s.ring[s.hand] = cacheSlot{page: page, n: n}
	s.index[page] = s.hand
	s.hand = (s.hand + 1) % len(s.ring)
}

// drop removes freed pages.
func (c *cache) drop(start, n uint64) {
	if r := c.root.Load(); r != nil && r.page >= start && r.page < start+n {
		c.root.CompareAndSwap(r, nil)
	}
	for p := start; p < start+n; p++ {
		s := c.shard(p)
		s.mtx.Lock()
		if i, ok := s.index[p]; ok {
			delete(s.index, p)
			s.ring[i] = cacheSlot{}
		}
		s.mtx.Unlock()
	}
}
