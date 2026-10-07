package manifest

import (
	"testing"

	"github.com/s4wave/spacewave/db/packfile"
)

// TestManifestDeltaIndexEviction preserves cached indexes only for unchanged contents.
func TestManifestDeltaIndexEviction(t *testing.T) {
	// Seed two accepted entries and their durable index tails.
	ctx := t.Context()
	backend := newTestStore()
	catalog, err := New(ctx, backend)
	if err != nil {
		t.Fatal(err)
	}
	cache := NewIndexCache(backend)
	initial := []*packfile.PackfileEntry{
		{Id: "changed", Sequence: 1, SizeBytes: 10, BlockCount: 1},
		{Id: "kept", Sequence: 2, SizeBytes: 20, BlockCount: 2},
	}
	if err := catalog.ApplyDelta(ctx, initial, nil, 2); err != nil {
		t.Fatal(err)
	}
	for _, entry := range initial {
		if err := cache.Set(ctx, entry.GetId(), []byte("index")); err != nil {
			t.Fatal(err)
		}
	}

	// Sequence-only publication and rejected local metadata retain both tails.
	sequenced := initial[0].CloneVT()
	sequenced.Sequence = 3
	if err := catalog.ApplyDelta(ctx, []*packfile.PackfileEntry{sequenced, {Id: "kept", SizeBytes: 99}}, nil, 3); err != nil {
		t.Fatal(err)
	}
	for _, entry := range initial {
		if _, found, err := cache.Get(ctx, entry.GetId()); err != nil || !found {
			t.Fatalf("unchanged index %s: found=%v err=%v", entry.GetId(), found, err)
		}
	}

	// Accept changed contents and discard only that pack's index.
	changed := sequenced.CloneVT()
	changed.Sequence = 4
	changed.BlockCount = 2
	if err := catalog.ApplyDelta(ctx, []*packfile.PackfileEntry{changed}, nil, 4); err != nil {
		t.Fatal(err)
	}
	if _, found, err := cache.Get(ctx, "changed"); err != nil || found {
		t.Fatalf("changed index survived: found=%v err=%v", found, err)
	}
	if _, found, err := cache.Get(ctx, "kept"); err != nil || !found {
		t.Fatalf("unaffected index lost: found=%v err=%v", found, err)
	}
}
