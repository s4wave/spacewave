package store

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/tidwall/btree"
)

// ManifestSnapshot pins catalog membership without copying retained entries.
// It borrows the store's lifetime and its shared reader/index caches.
type ManifestSnapshot struct {
	// store owns readers and their lifetime.
	store *PackfileStore
	// entries is an immutable copy-on-write tree in lookup order.
	entries *btree.BTreeG[*manifestEntry]
}

// SnapshotManifest captures the current catalog in constant space and time.
// The result is safe for concurrent reads until the store is closed.
func (s *PackfileStore) SnapshotManifest() *ManifestSnapshot {
	var entries *btree.BTreeG[*manifestEntry]
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		entries = s.manifest.Copy()
	})
	return &ManifestSnapshot{store: s, entries: entries}
}

// GetEntries materializes an explicitly requested complete catalog snapshot.
func (v *ManifestSnapshot) GetEntries() []*packfile.PackfileEntry {
	entries := make([]*packfile.PackfileEntry, 0, v.entries.Len())
	v.entries.Scan(func(item *manifestEntry) bool {
		entries = append(entries, item.entry)
		return true
	})
	return entries
}

// WithoutTrash returns the snapshot without its trash packs, so an upload
// deduplicates only against packs a reclaim pass will not retire.
func (v *ManifestSnapshot) WithoutTrash() *ManifestSnapshot {
	// Collect the trash packs.
	var trash []*manifestEntry
	v.entries.Scan(func(item *manifestEntry) bool {
		if item.entry.IsTrash() {
			trash = append(trash, item)
		}
		return true
	})
	if len(trash) == 0 {
		return v
	}

	// Drop them from a copy of the tree.
	entries := v.entries.Copy()
	for _, item := range trash {
		entries.Delete(item)
	}
	return &ManifestSnapshot{store: v.store, entries: entries}
}

// GetBlockExistsBatch probes only packs present when the snapshot was captured.
// Paging a flush therefore never fetches indexes of packs it just uploaded.
func (v *ManifestSnapshot) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return v.store.getBlockExistsBatch(ctx, v, refs)
}
