package s4db

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"slices"
	"sync/atomic"

	"github.com/pkg/errors"
)

// Page layout: kind byte, entry count, entries, zero padding, and a CRC-32C of
// the rest in the last four bytes.
const (
	// pageHeader is the kind byte and the two-byte count.
	pageHeader = 3
	// pageRoom is the space for entries.
	pageRoom = pageSize - pageHeader - 4
	// maxInline is the largest inline value; a leaf entry holding it and
	// the longest key fits in a page.
	maxInline = 1024
)

// Page kinds.
const (
	// pageInner holds keys and child pages.
	pageInner = 0
	// pageLeaf holds keys and values.
	pageLeaf = 1
)

// Decoded entry costs in memory on a 64-bit platform, beyond the page bytes
// the node keeps. A slice header is 24 bytes and a value 48.
const (
	// leafEntryCost is a key slice, its head, and its value.
	leafEntryCost = 24 + 8 + 48
	// innerEntryCost is a key slice, its head, its child page, and its
	// child link.
	innerEntryCost = 24 + 8 + 8 + 8
)

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
	// cost is the memory the decoded node holds, for the cache budget.
	cost int
}

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
			continue
		}
		hi = m
	}
	return lo, lo < len(n.keys) && bytes.Equal(n.keys[lo], key)
}

// childIndex returns the child of inner page n covering key.
func (n *node) childIndex(key []byte) int {
	i, found := n.search(key)
	if found {
		return i
	}
	return max(0, i-1)
}

// fill returns the bytes the entries of n occupy.
func (n *node) fill() int {
	total := 0
	for i, k := range n.keys {
		if n.leaf {
			total += leafEntrySize(k, n.vals[i])
			continue
		}
		total += innerEntrySize(k)
	}
	return total
}

// leafEntrySize returns the encoded length of a leaf entry.
func leafEntrySize(key []byte, v value) int {
	return uvarintLen(uint64(len(key))) + len(key) + v.size()
}

// innerEntrySize returns the encoded length of an inner entry.
func innerEntrySize(key []byte) int {
	return uvarintLen(uint64(len(key))) + len(key) + 8
}

// encode appends the page holding n to dst.
func (n *node) encode(dst []byte) []byte {
	// Write the kind and count.
	start := len(dst)
	b := slices.Grow(dst, pageSize)[:start+pageHeader]
	b[start] = pageInner
	if n.leaf {
		b[start] = pageLeaf
	}
	binary.LittleEndian.PutUint16(b[start+1:], uint16(len(n.keys))) // #nosec G115 -- a page holds under 1<<16 entries.

	// Write each key with its value or child page.
	for i, k := range n.keys {
		b = binary.AppendUvarint(b, uint64(len(k)))
		b = append(b, k...)
		if n.leaf {
			b = n.vals[i].append(b)
			continue
		}
		b = binary.LittleEndian.AppendUint64(b, n.kids[i])
	}

	// Pad the page and seal it.
	end := len(b)
	b = b[:start+pageSize]
	clear(b[end:])
	page := b[start:]
	binary.LittleEndian.PutUint32(page[pageSize-4:], checksum(page[:pageSize-4]))
	return b
}

// decodeNode reads a page, keeping references into b.
func decodeNode(b []byte) (*node, error) {
	// Check the page checksum and kind.
	if checksum(b[:pageSize-4]) != binary.LittleEndian.Uint32(b[pageSize-4:]) {
		return nil, errors.Wrap(ErrCorrupt, "page checksum mismatch")
	}
	if b[0] != pageInner && b[0] != pageLeaf {
		return nil, errors.Wrap(ErrCorrupt, "unknown page kind")
	}

	// Size the node for its kind and count.
	count := int(binary.LittleEndian.Uint16(b[1:]))
	n := &node{leaf: b[0] == pageLeaf, keys: make([][]byte, count), heads: make([]uint64, count)}
	d := decoder{b: b[pageHeader : pageSize-4]}
	if n.leaf {
		n.vals = make([]value, count)
		n.cost = pageSize + count*leafEntryCost
	} else {
		n.kids = make([]uint64, count)
		n.inner = make([]atomic.Pointer[node], count)
		n.cost = pageSize + count*innerEntryCost
	}

	// Read each key with its value or child page.
	for i := range count {
		n.keys[i] = d.bytes()
		n.heads[i] = head(n.keys[i])
		if n.leaf {
			n.vals[i] = d.value(d.kind())
			continue
		}
		n.kids[i] = d.u64()
	}
	return n, d.err
}
