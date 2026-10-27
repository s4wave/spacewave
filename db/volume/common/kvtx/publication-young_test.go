package kvtx

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestYoungReadySurvivesReopen checks that minor passes run on a store born
// empty and on one a block pass promoted before a restart, never on a store
// upgraded from a format whose builds may not log young records, and that a
// minor pass sweeps only released blocks.
func TestYoungReadySurvivesReopen(t *testing.T) {
	ctx := t.Context()
	store := store_kvtx_inmem.NewStore()
	keys := store_kvkey.NewDefaultKVKey()
	open := func() *Volume {
		t.Helper()
		v, err := NewVolume(ctx, "test/young", keys, store, &store_kvtx.Config{}, false, false, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	closeVolume := func(v *Volume) {
		t.Helper()
		if err := v.Close(); err != nil {
			t.Fatal(err)
		}
	}
	sweepYoung := func(v *Volume) {
		t.Helper()
		var stats block_gc.Stats
		if err := v.SweepYoungBlocks(ctx, v.GetRefGraph(), &stats); err != nil {
			t.Fatal(err)
		}
	}

	// A store born empty is minor-ready: a minor pass sweeps a released
	// block and keeps a published one.
	v := open()
	live := publicationFor(t, "live", "", "one")
	if err := v.PublishAtomic(ctx, live); err != nil {
		t.Fatal(err)
	}
	first := publicationFor(t, "first", "", "one")
	if err := v.PublishAtomic(ctx, first); err != nil {
		t.Fatal(err)
	}
	orphanPublication(t, v, first)
	sweepYoung(v)
	assertPublishedBlock(t, v, first, false)
	assertPublishedBlock(t, v, live, true)
	closeVolume(v)

	// Stand in for a store of format version 4 with blocks: drop the ready
	// record, 'm' in the reference graph, and rewind the version.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Delete(ctx, append(append([]byte(nil), refGraphPrefix...), 'm')); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(4)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// The upgraded store is not minor-ready: a minor pass sweeps nothing.
	v = open()
	second := publicationFor(t, "second", "", "one")
	if err := v.PublishAtomic(ctx, second); err != nil {
		t.Fatal(err)
	}
	orphanPublication(t, v, second)
	sweepYoung(v)
	if _, found, err := v.GetBlock(ctx, second.Entries[0].Ref); err != nil || !found {
		t.Fatalf("minor pass on an upgraded store: found=%v err=%v", found, err)
	}

	// A major pass sweeps it and promotes.
	var stats block_gc.Stats
	if err := v.SweepUnreachedBlocks(ctx, v.GetRefGraph(), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.BlocksSwept != 1 {
		t.Fatalf("major pass stats=%+v", stats)
	}
	assertPublishedBlock(t, v, live, true)
	closeVolume(v)

	// After a restart minor passes run, and keep the live block.
	v = open()
	t.Cleanup(func() { closeVolume(v) })
	third := publicationFor(t, "third", "", "one")
	if err := v.PublishAtomic(ctx, third); err != nil {
		t.Fatal(err)
	}
	orphanPublication(t, v, third)
	sweepYoung(v)
	assertPublishedBlock(t, v, third, false)
	assertPublishedBlock(t, v, live, true)
}

// TestMinorPassKeepsFilledHole checks that a minor pass keeps a block a live
// parent names when a major pass settled it as absent and a cache fill stores
// it later. The parent is old and its list did not change, so no young record
// names the edge, and the filled block has no owner but the staging edge.
func TestMinorPassKeepsFilledHole(t *testing.T) {
	// Publish and release a block, and publish a parent whose list names a
	// block the store does not hold.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	rg := v.GetRefGraph()
	released := publicationFor(t, "released", "", "one")
	if err := v.PublishAtomic(ctx, released); err != nil {
		t.Fatal(err)
	}
	orphanPublication(t, v, released)
	parent := publicationFor(t, "parent", "", "one")
	if err := v.PublishAtomic(ctx, parent); err != nil {
		t.Fatal(err)
	}
	filled := publicationFor(t, "hole", "", "two").Entries[0]
	hole := block_gc.BlockIRI(filled.Ref)
	edge := block_gc.RefEdge{Subject: block_gc.BlockIRI(parent.Entries[0].Ref), Object: hole}
	if err := rg.ApplyRefBatch(ctx, []block_gc.RefEdge{edge}, nil); err != nil {
		t.Fatal(err)
	}

	// A major pass settles the hole, sweeps the released block and promotes.
	var stats block_gc.Stats
	if err := v.SweepUnreachedBlocks(ctx, rg, &stats); err != nil {
		t.Fatal(err)
	}
	if stats.AbsentBlocks != 1 || stats.UnknownBlocks != 0 || stats.BlocksSwept != 1 {
		t.Fatalf("major pass stats=%+v", stats)
	}

	// A read-through fill stores the hole with its refs known.
	gs := block_gc.NewGCStoreOps(v, rg)
	opts := &block.PutOpts{ForceBlockRef: filled.Ref, CacheFill: true, RefsKnown: true}
	if _, _, err := gs.PutBlock(ctx, filled.Data, opts); err != nil {
		t.Fatal(err)
	}
	if err := gs.FlushPending(ctx); err != nil {
		t.Fatal(err)
	}

	// A minor pass keeps it, since the live parent names it.
	var minor block_gc.Stats
	if err := v.SweepYoungBlocks(ctx, rg, &minor); err != nil {
		t.Fatal(err)
	}
	if minor.MinorPasses != 1 {
		t.Fatalf("minor pass did not run: stats=%+v", minor)
	}
	if _, found, err := v.GetBlock(ctx, filled.Ref); err != nil || !found {
		t.Fatalf("filled block a live parent names: found=%v err=%v swept=%d", found, err, minor.BlocksSwept)
	}
	assertPublishedBlock(t, v, parent, true)
}
