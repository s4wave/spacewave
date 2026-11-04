package store

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"slices"

	"github.com/aperturerobotics/go-kvfile"
	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/hash"
)

// maxTailEntryBytes is the largest index entry go-kvfile reads.
const maxTailEntryBytes = 2048

// tailIndex is a blockIndex that searches the raw kvfile index tail in place.
//
// The tail is the layout go-kvfile writes after the payloads: every index
// entry as its protobuf message followed by its size varint, in key order;
// then a table of one little-endian uint64 per entry, the pack offset of that
// entry's size varint; then the entry count. The table makes entry i
// addressable without a scan, so the key binary search reads the tail bytes
// directly and a load allocates the tail, the offset order and nothing per
// entry. A dropped index is one slice to free.
//
// byOff holds the entry numbers in payload offset order, the one structure the
// tail does not hold. It is the permutation slices.SortFunc by offset gives
// the entries in key order, so ties order as the parsed index orders them.
type tailIndex struct {
	// tail is the raw index tail. The index owns it and never writes it.
	tail []byte
	// base is the pack offset of tail[0].
	base uint64
	// table is the tail offset of the entry position table.
	table int
	// count is the number of entries.
	count int
	// byOff lists the entry numbers by increasing payload offset.
	byOff []uint32
}

// newTailIndex validates an index tail of a pack of size bytes against the
// manifest block count and builds the in-place index over it. tail must be
// the exact tail, owned by the caller and kept by the index.
//
// A tail is valid exactly when the parsed index accepts it: go-kvfile accepts
// the layout, every entry reads and decodes, the count is blockCount, and
// validateIndexEntries accepts the entries. Each entry is decoded by both the
// in-place decoder and kvfile's own, so a lookup returns what the parsed
// index holds for the key by construction; any disagreement rejects the tail.
func newTailIndex(tail []byte, size uint64, blockCount uint64) (*tailIndex, error) {
	// Let go-kvfile validate the layout, as the parsed index does.
	reader, err := kvfile.BuildReaderWithIndexTail(tail, size)
	if err != nil {
		return nil, errors.Wrap(err, "build kvfile reader from index tail")
	}
	if reader.Size() != blockCount {
		return nil, errors.Errorf("index entry count %d != manifest block count %d", reader.Size(), blockCount)
	}
	if blockCount > math.MaxUint32 || blockCount > uint64(math.MaxInt) {
		return nil, errors.Errorf("index entry count %d overflows the index", blockCount)
	}
	// go-kvfile accepts an empty tail as a pack with no entries; it has no
	// count word, so there is no table to locate.
	if len(tail) == 0 {
		return &tailIndex{}, nil
	}
	t := &tailIndex{
		tail:  tail,
		base:  size - uint64(len(tail)),
		count: int(blockCount),
	}
	t.table = len(tail) - 8 - 8*t.count
	if t.table < 0 {
		return nil, errors.Errorf("index entry count %d too large for tail", t.count)
	}

	// Check every entry once, ordering them by payload offset on the way.
	type located struct {
		off uint64
		idx uint32
	}
	order := make([]located, t.count)
	chk := newIndexChecker(size-uint64(len(tail)), blockCount)
	var ref kvfile.IndexEntry
	for i := range t.count {
		raw, entryPos, err := t.rawEntry(i)
		if err != nil {
			return nil, err
		}
		key, off, esize, err := decodeIndexEntry(raw)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid index entry at %v", entryPos)
		}
		if off > entryPos {
			return nil, errors.Errorf("invalid index entry at %v: offset %v is greater than index entry pos", entryPos, off)
		}

		// Require kvfile's decoder to read the same entry.
		ref.Key, ref.Offset, ref.Size = ref.Key[:0], 0, 0
		if err := ref.UnmarshalVT(raw); err != nil {
			return nil, errors.Wrapf(err, "invalid index entry at %v", entryPos)
		}
		if !bytes.Equal(ref.Key, key) || ref.Offset != off || ref.Size != esize {
			return nil, errors.Errorf("index entry %d decodes differently in place", i)
		}

		if err := chk.check(i, key, off, esize); err != nil {
			return nil, err
		}
		order[i] = located{off: off, idx: uint32(i)} //nolint:gosec // count is at most MaxUint32.
	}

	// Order the entries by payload offset with the parsed index's comparison.
	slices.SortFunc(order, func(a, b located) int {
		return cmpUint64(a.off, b.off)
	})
	t.byOff = make([]uint32, t.count)
	for i, o := range order {
		t.byOff[i] = o.idx
	}
	return t, nil
}

// cmpUint64 compares two offsets.
func cmpUint64(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// window returns the n tail bytes at pack offset pos, or an error when they
// are not all inside the tail, as reading the tail as a file does.
func (t *tailIndex) window(pos uint64, n int) ([]byte, error) {
	if pos < t.base {
		return nil, io.EOF
	}
	rel := pos - t.base
	if rel >= uint64(len(t.tail)) || uint64(n) > uint64(len(t.tail))-rel {
		return nil, io.ErrUnexpectedEOF
	}
	return t.tail[rel : rel+uint64(n)], nil
}

// rawEntry returns the protobuf bytes of entry i and their pack offset, with
// the checks go-kvfile makes reading an entry.
func (t *tailIndex) rawEntry(i int) ([]byte, uint64, error) {
	// Read the pack offset of the entry's size varint from the table.
	at := t.table + 8*i
	sizePos := binary.LittleEndian.Uint64(t.tail[at : at+8])

	// Read the size varint through a ten byte window, as go-kvfile does.
	buf, err := t.window(sizePos, 10)
	if err != nil {
		return nil, 0, errors.Wrapf(err, "read index entry %d size", i)
	}
	size, n := protobuf_go_lite.ConsumeVarint(buf)
	if n < 0 {
		return nil, 0, errors.Errorf("invalid index entry size varint at %v", sizePos)
	}
	if size > maxTailEntryBytes {
		return nil, 0, errors.Errorf("invalid index entry size at %v: %v > %v", sizePos, size, maxTailEntryBytes)
	}
	if sizePos < size {
		return nil, 0, errors.Errorf("invalid index entry size %v at %v: larger than its position", size, sizePos)
	}
	entryPos := sizePos - size
	raw, err := t.window(entryPos, int(size))
	if err != nil {
		return nil, 0, errors.Wrapf(err, "read index entry %d", i)
	}
	return raw, entryPos, nil
}

// entry returns the key, offset and size of entry i below count. The tail was
// validated, so a read cannot fail; an entry that cannot be read reads as
// empty and is never matched.
func (t *tailIndex) entry(i int) ([]byte, uint64, uint64) {
	raw, _, err := t.rawEntry(i)
	if err != nil {
		return nil, 0, 0
	}
	key, off, size, err := decodeIndexEntry(raw)
	if err != nil {
		return nil, 0, 0
	}
	return key, off, size
}

// extent returns the byte interval of entry i.
func (t *tailIndex) extent(i int) blockExtent {
	_, off, size := t.entry(i)
	o := int64(off)                                  //nolint:gosec // validateIndexEntries bounds offsets by the int64 pack size.
	return blockExtent{off: o, end: o + int64(size)} //nolint:gosec // validated entries have non-overflowing extents within the int64 pack size.
}

// cost implements blockIndex.
func (t *tailIndex) cost() int64 {
	return int64(len(t.tail)) + 4*int64(len(t.byOff))
}

// len implements blockIndex.
func (t *tailIndex) len() int {
	return t.count
}

// find implements blockIndex.
func (t *tailIndex) find(key []byte) (blockExtent, bool) {
	lo, hi := 0, t.count
	for lo < hi {
		h := int(uint(lo+hi) >> 1)
		k, _, _ := t.entry(h)
		if bytes.Compare(k, key) < 0 {
			lo = h + 1
		} else {
			hi = h
		}
	}
	if lo < t.count {
		if k, _, _ := t.entry(lo); bytes.Equal(k, key) {
			return t.extent(lo), true
		}
	}
	return blockExtent{}, false
}

// seekEnd implements blockIndex.
func (t *tailIndex) seekEnd(start int64) int {
	lo, hi := 0, t.count
	for lo < hi {
		h := int(uint(lo+hi) >> 1)
		if t.extent(int(t.byOff[h])).end > start {
			hi = h
		} else {
			lo = h + 1
		}
	}
	return lo
}

// seekOffset implements blockIndex.
func (t *tailIndex) seekOffset(off int64) int {
	lo, hi := 0, t.count
	for lo < hi {
		h := int(uint(lo+hi) >> 1)
		if t.extent(int(t.byOff[h])).off >= off {
			hi = h
		} else {
			lo = h + 1
		}
	}
	return lo
}

// atOffset implements blockIndex.
func (t *tailIndex) atOffset(pos int) ([]byte, blockExtent) {
	i := int(t.byOff[pos])
	key, _, _ := t.entry(i)
	return key, t.extent(i)
}

// decodeIndexEntry decodes a kvfile.IndexEntry message as its UnmarshalVT does,
// except that the key aliases b instead of copying it.
func decodeIndexEntry(b []byte) (key []byte, off, size uint64, err error) {
	idx := 0
	for idx < len(b) {
		pre := idx
		var wire uint64
		wire, idx, err = protobuf_go_lite.DecodeVarint(b, idx)
		if err != nil {
			return nil, 0, 0, err
		}
		fieldNum := int32(wire >> 3) //nolint:gosec // the field number is below 1<<29.
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return nil, 0, 0, errors.New("proto: IndexEntry: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return nil, 0, 0, errors.Errorf("proto: IndexEntry: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return nil, 0, 0, errors.Errorf("proto: wrong wireType = %d for field Key", wireType)
			}
			var start, end int
			start, end, err = protobuf_go_lite.DecodeLengthDelimited(b, idx)
			if err != nil {
				return nil, 0, 0, err
			}
			key = b[start:end:end]
			idx = end
		case 2:
			if wireType != 0 {
				return nil, 0, 0, errors.Errorf("proto: wrong wireType = %d for field Offset", wireType)
			}
			off, idx, err = protobuf_go_lite.DecodeVarint(b, idx)
			if err != nil {
				return nil, 0, 0, err
			}
		case 3:
			if wireType != 0 {
				return nil, 0, 0, errors.Errorf("proto: wrong wireType = %d for field Size", wireType)
			}
			size, idx, err = protobuf_go_lite.DecodeVarint(b, idx)
			if err != nil {
				return nil, 0, 0, err
			}
		default:
			skip, err := protobuf_go_lite.Skip(b[pre:])
			if err != nil {
				return nil, 0, 0, err
			}
			if skip < 0 || pre+skip < 0 {
				return nil, 0, 0, protobuf_go_lite.ErrInvalidLength
			}
			if pre+skip > len(b) {
				return nil, 0, 0, io.ErrUnexpectedEOF
			}
			idx = pre + skip
		}
	}
	return key, off, size, nil
}

// indexChecker validates index entries one at a time, in key order, against the
// pack: strictly increasing block keys, keys that name a block hash, and
// payloads that end before the index tail.
type indexChecker struct {
	// tailStart is the pack offset where the index tail begins.
	tailStart uint64
	// blockCount is the entry count the manifest declares.
	blockCount uint64
	// prev is the previous entry's key.
	prev []byte
	// h is scratch for parsing block keys.
	h hash.Hash
}

// newIndexChecker builds a checker for a pack whose index tail begins at
// tailStart.
func newIndexChecker(tailStart, blockCount uint64) *indexChecker {
	return &indexChecker{tailStart: tailStart, blockCount: blockCount}
}

// check validates entry i, which must follow entry i-1.
func (c *indexChecker) check(i int, key []byte, off, size uint64) error {
	// Require a nonempty key.
	if len(key) == 0 {
		return errors.Errorf("empty index key at %d", i)
	}

	// Require strictly increasing index keys that encode block references.
	if i != 0 && bytes.Compare(c.prev, key) >= 0 {
		return errors.Errorf("duplicate or unsorted index key at %d", i)
	}
	c.prev = key
	c.h.HashType, c.h.Hash = 0, c.h.Hash[:0]
	if err := c.h.UnmarshalVT(key); err != nil {
		return errors.Wrapf(err, "parse index key at %d", i)
	}
	if err := c.h.Validate(); err != nil {
		return errors.Wrapf(err, "parse index key at %d", i)
	}

	// Require each block payload to end before the index tail without overflow.
	if size > math.MaxUint64-off {
		return errors.Errorf("index entry %d offset %d size %d overflows", i, off, size)
	}
	if off+size > c.tailStart {
		return errors.Errorf("index entry %d end %d exceeds tail start %d", i, off+size, c.tailStart)
	}
	return nil
}

// _ is a type assertion
var (
	_ blockIndex = (*tailIndex)(nil)
	_ blockIndex = (*parsedIndex)(nil)
)
