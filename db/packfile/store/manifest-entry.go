package store

import (
	"cmp"

	"github.com/s4wave/spacewave/db/block/bloom"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/tidwall/btree"
)

// manifestEntry keeps an immutable pack descriptor with its parsed bloom filter.
type manifestEntry struct {
	// entry describes the accepted pack.
	entry *packfile.PackfileEntry
	// filter rejects absent keys; nil requires consulting the index.
	filter *bloom.Filter
}

// newManifestTree orders server sequences first, newest first, then IDs.
func newManifestTree() *btree.BTreeG[*manifestEntry] {
	return btree.NewBTreeGOptions(func(a, b *manifestEntry) bool {
		if order := cmp.Compare(b.entry.GetSequence(), a.entry.GetSequence()); order != 0 {
			return order < 0
		}
		return a.entry.GetId() < b.entry.GetId()
	}, btree.Options{NoLocks: true})
}

// parseManifestEntry copies an accepted descriptor and parses its bloom once.
func parseManifestEntry(entry *packfile.PackfileEntry) *manifestEntry {
	item := &manifestEntry{entry: entry.CloneVT()}
	var encoded bloom.BloomFilter
	if err := encoded.UnmarshalBlock(item.entry.GetBloomFilter()); err == nil {
		item.filter = encoded.ToBloomFilter()
	}
	return item
}
