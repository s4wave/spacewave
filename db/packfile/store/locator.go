package store

import (
	"context"

	"github.com/s4wave/spacewave/db/block/bloom"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/tidwall/btree"
)

// Locator places a block key in a pack without the bloom filters.
//
// The store consults it before the filters: a pack the locator Indexed is
// found through Locate alone, and only the packs it has not indexed still
// test their bloom filter. Once every pack of the manifest is indexed, a key
// Locate does not place is in no pack, so the lookup ends without opening one.
type Locator interface {
	// Locate returns the id of the pack that holds key, and false when no
	// indexed pack does. The pack may have left the manifest since.
	Locate(ctx context.Context, key []byte) (packID string, found bool, err error)
	// Indexed reports whether Locate knows every key of the pack.
	Indexed(packID string) bool
	// Version changes whenever Indexed may answer differently for some pack.
	// While a pack stays in the manifest, its answer turns only from false to
	// true: the store refilters only the packs it last found unindexed, so a
	// pack must leave the manifest before the locator forgets it.
	Version() uint64
}

// SetLocator sets the locator the lookups consult. Call it before the first
// lookup.
func (s *PackfileStore) SetLocator(l Locator) {
	s.locator = l
}

// unindexedPacks is a snapshot's catalog split by the locator at one version.
type unindexedPacks struct {
	// version is the locator version the split was computed at.
	version uint64
	// open holds the packs the locator had not indexed at version, in lookup
	// order. It may also hold packs indexed since, which cost a filter test.
	// It is immutable: a newer split copies it.
	open *btree.BTreeG[*manifestEntry]
}

// derive returns the split of a snapshot that dropped and added packs to the
// snapshot this split covers. An added pack is open until the next refilter.
func (p *unindexedPacks) derive(dropped, added []*manifestEntry) *unindexedPacks {
	open := p.open.Copy()
	for _, item := range dropped {
		open.Delete(item)
	}
	for _, item := range added {
		open.Set(item)
	}
	return &unindexedPacks{version: p.version, open: open}
}

// unindexedPacks returns the snapshot's packs the locator has not indexed,
// computing the split once per snapshot and locator version; concurrent
// lookups wait for the one computing it. A newer version refilters only the
// packs the last split left open, since an indexed pack stays indexed while it
// is in the manifest; a snapshot without a split scans the whole catalog. The
// version is read before the split, so a change during it makes the next
// lookup recompute.
func (v *ManifestSnapshot) unindexedPacks(loc Locator) *unindexedPacks {
	// Reuse the split while the locator answers as it did.
	version := loc.Version()
	if packs := v.unindexed.Load(); packs != nil && packs.version == version {
		return packs
	}
	v.unindexedMtx.Lock()
	defer v.unindexedMtx.Unlock()
	prev := v.unindexed.Load()
	if prev != nil && prev.version >= version {
		return prev
	}

	// Drop the packs indexed since the last split, or collect the open packs
	// of the whole catalog when there is none.
	packs := &unindexedPacks{version: version}
	if prev != nil {
		packs.open = prev.open.Copy()
		prev.open.Scan(func(item *manifestEntry) bool {
			if loc.Indexed(item.entry.GetId()) {
				packs.open.Delete(item)
			}
			return true
		})
	} else {
		packs.open = newManifestTree()
		v.entries.Scan(func(item *manifestEntry) bool {
			if !loc.Indexed(item.entry.GetId()) {
				packs.open.Set(item)
			}
			return true
		})
	}
	v.unindexed.Store(packs)
	return packs
}

// entryByID returns the snapshot's entry for pack id, or nil.
func (v *ManifestSnapshot) entryByID(id string) *manifestEntry {
	item, _ := v.ids.Get(&manifestEntry{entry: &packfile.PackfileEntry{Id: id}})
	return item
}

// candidates returns the packs of view that may hold key, the pack the locator
// places it in first.
func (s *PackfileStore) candidates(ctx context.Context, view *ManifestSnapshot, key []byte) ([]*manifestEntry, error) {
	// Without a locator every pack tests its filter.
	bloomKey := bloom.NewKey(key)
	loc := s.locator
	if loc == nil {
		return view.filterCandidates(bloomKey), nil
	}

	// Ask the locator which pack holds the key.
	located, found, err := loc.Locate(ctx, key)
	if err != nil {
		return nil, err
	}

	// Place the located pack first. When it left the manifest, the index
	// cannot rule out the packs it covers: test every filter.
	packs := view.unindexedPacks(loc)
	var out []*manifestEntry
	if found {
		hit := view.entryByID(located)
		if hit == nil {
			return view.filterCandidates(bloomKey), nil
		}
		out = append(out, hit)
	}

	// Test the filters of the packs the locator has not indexed.
	packs.open.Scan(func(item *manifestEntry) bool {
		if found && item == out[0] {
			return true
		}
		if item.filter == nil || item.filter.TestKey(bloomKey) {
			out = append(out, item)
		}
		return true
	})
	return out, nil
}

// filterCandidates returns every pack of view whose filter may hold key.
func (v *ManifestSnapshot) filterCandidates(key bloom.Key) []*manifestEntry {
	var out []*manifestEntry
	v.entries.Scan(func(item *manifestEntry) bool {
		if item.filter == nil || item.filter.TestKey(key) {
			out = append(out, item)
		}
		return true
	})
	return out
}
