package store

import (
	"context"

	"github.com/s4wave/spacewave/db/block/bloom"
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
}

// SetLocator sets the locator the lookups consult. Call it before the first
// lookup.
func (s *PackfileStore) SetLocator(l Locator) {
	s.locator = l
}

// candidates returns the packs of view that may hold key, the pack the locator
// places it in first.
func (s *PackfileStore) candidates(ctx context.Context, view *ManifestSnapshot, key []byte) ([]*manifestEntry, error) {
	// Ask the locator which pack holds the key.
	loc := s.locator
	var located string
	var found bool
	if loc != nil {
		var err error
		located, found, err = loc.Locate(ctx, key)
		if err != nil {
			return nil, err
		}
	}

	// Test the filters of the packs the locator has not indexed.
	bloomKey := bloom.NewKey(key)
	var hit *manifestEntry
	var out []*manifestEntry
	view.entries.Scan(func(item *manifestEntry) bool {
		id := item.entry.GetId()
		switch {
		case found && id == located:
			hit = item
		case loc != nil && loc.Indexed(id):
		case item.filter == nil || item.filter.TestKey(bloomKey):
			out = append(out, item)
		}
		return true
	})
	if hit != nil {
		return append([]*manifestEntry{hit}, out...), nil
	}
	if !found {
		return out, nil
	}

	// The located pack left the manifest, so the index cannot rule out the
	// packs it covers: test every filter.
	out = out[:0]
	view.entries.Scan(func(item *manifestEntry) bool {
		if item.filter == nil || item.filter.TestKey(bloomKey) {
			out = append(out, item)
		}
		return true
	})
	return out, nil
}
