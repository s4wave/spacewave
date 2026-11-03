package store

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/packfile"
)

// testLocator indexes the packs it is told to.
type testLocator struct {
	mtx     sync.Mutex
	indexed map[string]bool
	version atomic.Uint64
}

func (l *testLocator) Locate(context.Context, []byte) (string, bool, error) {
	return "", false, nil
}

func (l *testLocator) Indexed(id string) bool {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	return l.indexed[id]
}

func (l *testLocator) Version() uint64 { return l.version.Load() }

func (l *testLocator) index(ids ...string) {
	l.mtx.Lock()
	for _, id := range ids {
		l.indexed[id] = true
	}
	l.mtx.Unlock()
	l.version.Add(1)
}

// TestUnindexedPacksFollowManifest walks the locator split through manifest
// deltas, replaced descriptors and indexing, and checks each split against a
// scan of the whole catalog: a derived split must never miss an unindexed
// pack, or a lookup would skip the pack that holds its block.
func TestUnindexedPacksFollowManifest(t *testing.T) {
	loc := &testLocator{indexed: make(map[string]bool)}
	store := NewPackfileStore(nil, nil)
	store.SetLocator(loc)
	t.Cleanup(store.Close)

	// open returns the ids the split leaves open, and checks them against a
	// fresh scan of the snapshot.
	open := func() []string {
		t.Helper()
		view := store.SnapshotManifest()
		var got, want []string
		view.unindexedPacks(loc).open.Scan(func(item *manifestEntry) bool {
			if !loc.Indexed(item.entry.GetId()) {
				got = append(got, item.entry.GetId())
			}
			return true
		})
		view.entries.Scan(func(item *manifestEntry) bool {
			if !loc.Indexed(item.entry.GetId()) {
				want = append(want, item.entry.GetId())
			}
			return true
		})
		if !slices.Equal(got, want) {
			t.Fatalf("split leaves %v open, catalog has %v unindexed", got, want)
		}
		return got
	}
	entry := func(id string, seq uint64) *packfile.PackfileEntry {
		return &packfile.PackfileEntry{Id: id, Sequence: seq, SizeBytes: 10, BlockCount: 1}
	}

	store.ApplyManifestDelta([]*packfile.PackfileEntry{entry("a", 1), entry("b", 2)}, nil)
	if got := open(); len(got) != 2 {
		t.Fatalf("open %v, want both packs", got)
	}

	// Indexing refilters the open packs; a new snapshot inherits the split.
	loc.index("a")
	open()
	store.ApplyManifestDelta([]*packfile.PackfileEntry{entry("c", 3)}, nil)
	if got := open(); !slices.Equal(got, []string{"c", "b"}) {
		t.Fatalf("open %v, want c and b", got)
	}

	// A new descriptor with another sequence moves the pack in lookup order,
	// and a removed pack leaves the split.
	store.ApplyManifestDelta([]*packfile.PackfileEntry{entry("b", 4)}, []string{"c"})
	if got := open(); !slices.Equal(got, []string{"b"}) {
		t.Fatalf("open %v, want b", got)
	}

	// The split derived before indexing still holds the newly indexed pack
	// until the version check refilters it.
	store.ApplyManifestDelta([]*packfile.PackfileEntry{entry("d", 5)}, nil)
	loc.index("b", "d")
	if got := open(); len(got) != 0 {
		t.Fatalf("open %v, want none", got)
	}
	store.ApplyManifestDelta([]*packfile.PackfileEntry{entry("e", 6)}, []string{"a"})
	if got := open(); !slices.Equal(got, []string{"e"}) {
		t.Fatalf("open %v, want e", got)
	}
}
