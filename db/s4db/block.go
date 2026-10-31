package s4db

import (
	"bytes"
	"encoding/binary"

	"github.com/pkg/errors"
)

// A data block holds entries in key order with shared-prefix compression:
// uvarint(shared) uvarint(unshared) keySuffix value, a restart point every
// restartInterval entries where shared is zero, then a trailer of restart
// offsets. A value is a kind byte and its payload: inline bytes, a reference
// or nothing for a tombstone. The reference that locates a block carries its
// CRC-32C, which readChecked checks when the block leaves the file.

// offsets is a trailer of little-endian uint32 offsets read in place, as data
// blocks and index partitions end: the offsets, then their count.
type offsets []byte

// appendOffsets appends the trailer of offs to b.
func appendOffsets(b []byte, offs []uint32) []byte {
	for _, o := range offs {
		b = binary.LittleEndian.AppendUint32(b, o)
	}
	return binary.LittleEndian.AppendUint32(b, uint32(len(offs))) // #nosec G115 -- a block holds few offsets.
}

// cutOffsets splits b into the bytes before its trailer and the trailer's
// offsets, reporting false when the trailer does not fit in b.
func cutOffsets(b []byte) ([]byte, offsets, bool) {
	// Read the count and check the offsets fit before it.
	if len(b) < 4 {
		return nil, nil, false
	}
	n := uint64(binary.LittleEndian.Uint32(b[len(b)-4:]))
	if 4*n > uint64(len(b)-4) { // #nosec G115 -- len(b) is at least 4.
		return nil, nil, false
	}
	end := len(b) - 4 - int(4*n) // #nosec G115 -- bounded by len(b).
	return b[:end], offsets(b[end : len(b)-4]), true
}

// len returns the number of offsets.
func (o offsets) len() int {
	return len(o) / 4
}

// at returns offset i.
func (o offsets) at(i int) int {
	return int(binary.LittleEndian.Uint32(o[4*i:]))
}

// dataBlock is a data block read in place: its entries and the trailer of
// restart points.
type dataBlock struct {
	// data holds the entries, up to the trailer.
	data []byte
	// restarts holds the offset of each restart point.
	restarts offsets
}

// decodeBlock parses the block b in place; b must not change while the block
// is in use.
func decodeBlock(b []byte) (dataBlock, error) {
	data, restarts, ok := cutOffsets(b)
	if !ok || restarts.len() == 0 {
		return dataBlock{}, errors.Wrap(ErrCorrupt, "block restart count")
	}
	return dataBlock{data: data, restarts: restarts}, nil
}

// blockEntry is one entry decoded from a block.
type blockEntry struct {
	// key is the full key.
	key []byte
	// val is the value.
	val value
	// next is the offset of the entry after it.
	next int
}

// entryAt decodes the entry at off whose key shares prefix with the entry
// before it; prev is that entry's key.
func (blk *dataBlock) entryAt(off int, prev []byte) (blockEntry, bool) {
	// Read the lengths and check them against the input.
	if off > len(blk.data) {
		return blockEntry{}, false
	}
	d := decoder{b: blk.data[off:]}
	shared, unshared := d.uvarint(), d.uvarint()
	if d.err != nil || shared > uint64(len(prev)) || unshared > uint64(len(d.b)) {
		return blockEntry{}, false
	}

	// Build the key from the shared prefix and the suffix, then the value.
	key := make([]byte, 0, shared+unshared)
	key = append(key, prev[:shared]...)
	key = append(key, d.b[:unshared]...)
	d.b = d.b[unshared:]
	v := d.decodeValue(d.kind())
	if d.err != nil {
		return blockEntry{}, false
	}
	return blockEntry{key: key, val: v, next: len(blk.data) - len(d.b)}, true
}

// seek returns the first entry at or after key, reporting whether one
// exists.
func (blk *dataBlock) seek(key []byte) (blockEntry, bool) {
	// Bisect the restart points to the last one at or before key.
	lo, hi := 0, blk.restarts.len()
	for lo+1 < hi {
		m := int(uint(lo+hi) >> 1)
		e, ok := blk.entryAt(blk.restarts.at(m), nil)
		if !ok {
			return blockEntry{}, false
		}
		if bytes.Compare(e.key, key) <= 0 {
			lo = m
		} else {
			hi = m
		}
	}

	// Walk forward from it.
	off := blk.restarts.at(lo)
	var prev []byte
	for off < len(blk.data) {
		e, ok := blk.entryAt(off, prev)
		if !ok {
			return blockEntry{}, false
		}
		if bytes.Compare(e.key, key) >= 0 {
			return e, true
		}
		prev, off = e.key, e.next
	}
	return blockEntry{}, false
}

// blockBuilder writes a data block.
type blockBuilder struct {
	// buf holds the encoded entries.
	buf []byte
	// restarts holds the restart offsets.
	restarts []uint32
	// last is the last key added.
	last []byte
	// count counts the entries since the last restart.
	count int
	// entries counts the entries in the block.
	entries int
}

// add appends key with value v. Keys arrive in ascending order.
func (bb *blockBuilder) add(key []byte, v value) {
	// Start a restart point every restartInterval entries.
	shared := 0
	if bb.count == restartInterval || bb.entries == 0 {
		bb.restarts = append(bb.restarts, uint32(len(bb.buf))) // #nosec G115 -- a block is far below 4 GiB.
		bb.count = 0
	} else {
		for shared < len(key) && shared < len(bb.last) && key[shared] == bb.last[shared] {
			shared++
		}
	}

	// Write the entry.
	bb.buf = binary.AppendUvarint(bb.buf, uint64(shared))          // #nosec G115 -- a length.
	bb.buf = binary.AppendUvarint(bb.buf, uint64(len(key)-shared)) // #nosec G115 -- shared is at most len(key).
	bb.buf = append(bb.buf, key[shared:]...)
	bb.buf = v.append(bb.buf)
	bb.last = append(bb.last[:0], key...)
	bb.count++
	bb.entries++
}

// size returns the block's encoded length so far, trailer included.
func (bb *blockBuilder) size() int {
	return len(bb.buf) + 4*len(bb.restarts) + 4
}

// finish returns the encoded block and resets the builder.
func (bb *blockBuilder) finish() []byte {
	// Append the trailer of restart offsets.
	b := appendOffsets(bb.buf, bb.restarts)

	// Reset for the next block.
	*bb = blockBuilder{buf: make([]byte, 0, blockSize+blockSize/8), last: bb.last[:0]}
	return b
}
