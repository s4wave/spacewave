package store

import (
	"context"
	"sync"
	"sync/atomic"

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
	// ids is an immutable copy-on-write tree of the same entries in id order.
	ids *btree.BTreeG[*manifestEntry]
	// unindexed caches the packs the locator has not indexed, for the
	// locator version it was computed at. unindexedMtx lets one lookup
	// compute a split while the others wait for it.
	unindexed    atomic.Pointer[unindexedPacks]
	unindexedMtx sync.Mutex
}

// SnapshotManifest returns the current catalog in constant time. The result
// is immutable and safe for concurrent reads until the store is closed.
func (s *PackfileStore) SnapshotManifest() *ManifestSnapshot {
	return s.current.Load()
}

// publishManifest publishes a snapshot of the manifest. Call with bcast held,
// or before the store is shared.
func (s *PackfileStore) publishManifest() {
	s.current.Store(&ManifestSnapshot{store: s, entries: s.manifest.Copy(), ids: s.manifestIDs.Copy()})
}

// publishManifestDelta publishes a snapshot of the manifest that dropped and
// added the given entries to the current one, carrying its locator split
// across so a lookup need not scan the whole catalog. Call with bcast held.
func (s *PackfileStore) publishManifestDelta(dropped, added []*manifestEntry) {
	prev := s.current.Load()
	s.publishManifest()
	if packs := prev.unindexed.Load(); packs != nil {
		s.current.Load().unindexed.Store(packs.derive(dropped, added))
	}
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
