package store

import (
	"bytes"
	"cmp"
	"os"
	"slices"
	"sort"

	"github.com/aperturerobotics/go-kvfile"
)

// parsedIndexEnv names the environment variable that selects the parsed pack
// index. Any nonempty value makes every store load indexes as parsed
// kvfile.IndexEntry slices, the layout used before indexes were searched in
// place. SetParsedIndex overrides it per store.
const parsedIndexEnv = "SPACEWAVE_PACKFILE_PARSED_INDEX"

// parsedIndexDefault is whether stores start on the parsed index layout.
var parsedIndexDefault = os.Getenv(parsedIndexEnv) != ""

// parsedEntryBytes is the budget charge of one parsed index entry: the entry,
// its key and the two pointers that order it, about 128 MiB per 1<<20 entries.
const parsedEntryBytes = 128

// blockExtent is the packfile byte interval [off, end) of one block value.
type blockExtent struct {
	off, end int64
}

// blockIndex is the validated index of one pack: a block key to value extent
// lookup, and the entries in payload offset order for window planning.
//
// Invariant: both implementations hold exactly the entries of the pack's
// kvfile index tail, so find returns the extent of the entry the parsed index
// holds for the key and reports not found exactly when it has none, and the
// offset order is the order slices.SortFunc by offset gives the entries in key
// order.
//
// Methods are called under the reader lock and never mutate the index.
type blockIndex interface {
	// cost returns the resident bytes charged to the index budget.
	cost() int64
	// find returns the extent of the block stored under key.
	find(key []byte) (blockExtent, bool)
	// len returns the number of blocks in the index.
	len() int
	// seekEnd returns the first offset-order position whose extent ends after
	// start, or len() when none does.
	seekEnd(start int64) int
	// seekOffset returns the first offset-order position at or after off, or
	// len() when none is.
	seekOffset(off int64) int
	// atOffset returns the block key and extent at an offset-order position
	// below len(). The key is read-only and valid while the index is.
	atOffset(pos int) ([]byte, blockExtent)
}

// parsedIndex is a blockIndex over parsed kvfile entries: one sorted by key
// and one sorted by payload offset, each a slice of pointers to the same
// entries.
type parsedIndex struct {
	byOff []*kvfile.IndexEntry
	byKey []*kvfile.IndexEntry
}

// newParsedIndex orders entries, which are in key order, by key and by offset.
func newParsedIndex(entries []*kvfile.IndexEntry) *parsedIndex {
	byOff := slices.Clone(entries)
	slices.SortFunc(byOff, func(a, b *kvfile.IndexEntry) int {
		return cmp.Compare(a.GetOffset(), b.GetOffset())
	})
	byKey := slices.Clone(entries)
	slices.SortFunc(byKey, func(a, b *kvfile.IndexEntry) int {
		return bytes.Compare(a.GetKey(), b.GetKey())
	})
	return &parsedIndex{byOff: byOff, byKey: byKey}
}

// cost implements blockIndex.
func (p *parsedIndex) cost() int64 {
	return int64(len(p.byKey)) * parsedEntryBytes
}

// len implements blockIndex.
func (p *parsedIndex) len() int {
	return len(p.byKey)
}

// find implements blockIndex.
func (p *parsedIndex) find(key []byte) (blockExtent, bool) {
	i := sort.Search(len(p.byKey), func(i int) bool {
		return bytes.Compare(p.byKey[i].GetKey(), key) >= 0
	})
	if i < len(p.byKey) && bytes.Equal(p.byKey[i].GetKey(), key) {
		return entryExtent(p.byKey[i]), true
	}
	return blockExtent{}, false
}

// seekEnd implements blockIndex.
func (p *parsedIndex) seekEnd(start int64) int {
	return sort.Search(len(p.byOff), func(i int) bool {
		return entryExtent(p.byOff[i]).end > start
	})
}

// seekOffset implements blockIndex.
func (p *parsedIndex) seekOffset(off int64) int {
	return sort.Search(len(p.byOff), func(i int) bool {
		return entryExtent(p.byOff[i]).off >= off
	})
}

// atOffset implements blockIndex.
func (p *parsedIndex) atOffset(pos int) ([]byte, blockExtent) {
	return p.byOff[pos].GetKey(), entryExtent(p.byOff[pos])
}

// entryExtent returns the packfile byte interval of a validated index entry.
func entryExtent(entry *kvfile.IndexEntry) blockExtent {
	off := int64(entry.GetOffset())                                 //nolint:gosec // validateIndexEntries bounds offsets by the int64 pack size.
	return blockExtent{off: off, end: off + int64(entry.GetSize())} //nolint:gosec // validated entries have non-overflowing extents within the int64 pack size.
}
