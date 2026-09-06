//go:build js

package blockshard

import "testing"

// TestManifestSnapshotIsolation preserves retained generations and the public
// API's mutable-copy contract across publication and stale refresh results.
func TestManifestSnapshotIsolation(t *testing.T) {
	// Install an active and retired segment with independently mutable key ranges.
	s := &Shard{cache: newCacheCoordinator(0, 0)}
	s.mu.Lock()
	s.setManifestLocked(&Manifest{
		Generation: 1,
		Segments:   []SegmentMeta{{Filename: "active.sst", MinKey: []byte("a"), MaxKey: []byte("z")}},
		PendingDelete: []RetiredSegmentMeta{{
			SegmentMeta:      SegmentMeta{Filename: "retired.sst", MinKey: []byte("b"), MaxKey: []byte("y")},
			RetireGeneration: 1,
		}},
	})
	s.mu.Unlock()
	retained := s.manifestSnapshot()

	// Public callers can modify every returned range without affecting readers.
	public := s.Manifest()
	public.Generation = 99
	public.Segments[0].MinKey[0] = 'c'
	public.Segments[0].MaxKey[0] = 'x'
	public.PendingDelete[0].MinKey[0] = 'd'
	public.PendingDelete[0].MaxKey[0] = 'w'
	if retained.Generation != 1 || string(retained.Segments[0].MinKey) != "a" || string(retained.Segments[0].MaxKey) != "z" {
		t.Fatal("public manifest mutation changed an active snapshot")
	}
	if string(retained.PendingDelete[0].MinKey) != "b" || string(retained.PendingDelete[0].MaxKey) != "y" {
		t.Fatal("public manifest mutation changed retired key ranges")
	}

	// Writers clone before publication, leaving an in-flight reader's view intact.
	next := retained.Clone()
	next.Generation = 2
	next.Segments[0].MinKey[0] = 'e'
	next.PendingDelete[0].MinKey[0] = 'f'
	s.mu.Lock()
	s.setManifestLocked(next)
	s.mu.Unlock()
	current := s.manifestSnapshot()
	if current.Generation != 2 || string(current.Segments[0].MinKey) != "e" {
		t.Fatal("new reader did not observe the published generation")
	}
	if retained.Generation != 1 || string(retained.Segments[0].MinKey) != "a" || string(retained.PendingDelete[0].MinKey) != "b" {
		t.Fatal("publication changed a retained generation")
	}

	// A delayed disk-slot read cannot replace the newer installed generation.
	s.installObservedManifest(retained.Clone())
	if s.manifestSnapshot().Generation != 2 {
		t.Fatal("stale refresh replaced a newer generation")
	}
}
