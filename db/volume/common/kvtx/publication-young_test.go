package kvtx

import (
	"testing"
	"time"

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

// holeWriter stores a block and the child it names, child first, as one
// path that stores blocks does.
type holeWriter struct {
	name  string
	write func(t *testing.T, v *Volume, rg block_gc.RefGraphOps, child, parent *block.PutBatchEntry)
}

// holeWriters lists the writers that can store a hole: a read-through cache
// fill, a staged put no owner takes, and a batch put as a copy writes.
var holeWriters = []holeWriter{
	{"cache-fill", func(t *testing.T, v *Volume, rg block_gc.RefGraphOps, child, parent *block.PutBatchEntry) {
		gs := block_gc.NewGCStoreOps(v, rg)
		for _, e := range []*block.PutBatchEntry{child, parent} {
			opts := &block.PutOpts{ForceBlockRef: e.Ref, Refs: e.Refs, CacheFill: true, RefsKnown: true}
			if _, _, err := gs.PutBlock(t.Context(), e.Data, opts); err != nil {
				t.Fatal(err)
			}
		}
		if err := gs.FlushPending(t.Context()); err != nil {
			t.Fatal(err)
		}
	}},
	{"staged-put", func(t *testing.T, v *Volume, rg block_gc.RefGraphOps, child, parent *block.PutBatchEntry) {
		gs := block_gc.NewGCStoreOps(v, rg)
		for _, e := range []*block.PutBatchEntry{child, parent} {
			opts := &block.PutOpts{ForceBlockRef: e.Ref, Refs: e.Refs, RefsKnown: true}
			if _, _, err := gs.PutBlock(t.Context(), e.Data, opts); err != nil {
				t.Fatal(err)
			}
		}
		if err := gs.FlushPending(t.Context()); err != nil {
			t.Fatal(err)
		}
	}},
	{"batch-put", func(t *testing.T, v *Volume, rg block_gc.RefGraphOps, child, parent *block.PutBatchEntry) {
		gs := block_gc.NewGCStoreOps(v, rg)
		if _, err := gs.PutBlockBatch(t.Context(), []*block.PutBatchEntry{child, parent}); err != nil {
			t.Fatal(err)
		}
		if err := gs.FlushPending(t.Context()); err != nil {
			t.Fatal(err)
		}
	}},
}

// holeEntries returns a block that names a child, and the child, neither
// stored.
func holeEntries(t *testing.T) (hole, child *block.PutBatchEntry) {
	t.Helper()
	child = publicationFor(t, "hole-child", "", "two").Entries[0]
	hole = publicationFor(t, "hole", "", "two").Entries[0]
	hole.Refs, hole.RefsKnown = []*block.BlockRef{child.Ref}, true
	child.RefsKnown = true
	return hole, child
}

// TestMinorPassKeepsFilledHole checks that minor passes keep a block a live
// parent names, and the block it names, when a major pass settled it as
// absent and a write with no owner stores it later. The parent is old and its
// list did not change, and the filled block has no owner but the staging
// edge, so only the namer the settling logged leads a minor pass to it. Minor
// passes run before the fill too: each settles the hole again and logs its
// namer past its own promotion.
func TestMinorPassKeepsFilledHole(t *testing.T) {
	for _, w := range holeWriters {
		t.Run(w.name, func(t *testing.T) {
			// Publish and release a block, and publish a parent whose list
			// names a block the store does not hold.
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
			hole, child := holeEntries(t)
			edge := block_gc.RefEdge{Subject: block_gc.BlockIRI(parent.Entries[0].Ref), Object: block_gc.BlockIRI(hole.Ref)}
			if err := rg.ApplyRefBatch(ctx, []block_gc.RefEdge{edge}, nil); err != nil {
				t.Fatal(err)
			}

			// A major pass settles the hole, sweeps the released block and
			// promotes.
			var stats block_gc.Stats
			if err := v.SweepUnreachedBlocks(ctx, rg, &stats); err != nil {
				t.Fatal(err)
			}
			if stats.AbsentBlocks != 1 || stats.UnknownBlocks != 0 || stats.BlocksSwept != 1 {
				t.Fatalf("major pass stats=%+v", stats)
			}

			// Minor passes run while the hole is absent.
			minorPass := func(step string) {
				t.Helper()
				var minor block_gc.Stats
				if err := v.SweepYoungBlocks(ctx, rg, &minor); err != nil {
					t.Fatal(err)
				}
				if minor.MinorPasses != 1 {
					t.Fatalf("%s: minor pass did not run: stats=%+v", step, minor)
				}
				for _, e := range []*block.PutBatchEntry{hole, child} {
					if _, found, err := v.GetBlock(ctx, e.Ref); err != nil {
						t.Fatal(err)
					} else if !found && step != "before fill" {
						t.Fatalf("%s: block a live parent names swept: swept=%d", step, minor.BlocksSwept)
					}
				}
				assertPublishedBlock(t, v, parent, true)
			}
			// The first sweeps a released young block, so it settles the
			// hole in a sweep transaction and promotes past the record the
			// major pass wrote; the second has nothing unreached.
			churn := publicationFor(t, "churn", "", "one")
			if err := v.PublishAtomic(ctx, churn); err != nil {
				t.Fatal(err)
			}
			orphanPublication(t, v, churn)
			minorPass("before fill")
			assertPublishedBlock(t, v, churn, false)
			minorPass("before fill")

			// A write stores the hole, and minor passes keep it and its
			// child.
			w.write(t, v, rg, child, hole)
			minorPass("after fill")
			minorPass("after fill")

			// A major pass makes them old, and minor passes still keep them.
			stats = block_gc.Stats{}
			if err := v.SweepUnreachedBlocks(ctx, rg, &stats); err != nil {
				t.Fatal(err)
			}
			if stats.AbsentBlocks != 0 || stats.UnknownBlocks != 0 || stats.BlocksSwept != 0 {
				t.Fatalf("major pass after fill stats=%+v", stats)
			}
			minorPass("after major")
		})
	}
}

// TestMajorPassSettlesWithNothingUnreached checks that a major pass whose mark
// reaches a hole and leaves nothing unreached settles the hole and promotes,
// so minor passes run.
func TestMajorPassSettlesWithNothingUnreached(t *testing.T) {
	// Publish a parent whose list names a block the store does not hold.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	rg := v.GetRefGraph()
	parent := publicationFor(t, "parent", "", "one")
	if err := v.PublishAtomic(ctx, parent); err != nil {
		t.Fatal(err)
	}
	hole, _ := holeEntries(t)
	edge := block_gc.RefEdge{Subject: block_gc.BlockIRI(parent.Entries[0].Ref), Object: block_gc.BlockIRI(hole.Ref)}
	if err := rg.ApplyRefBatch(ctx, []block_gc.RefEdge{edge}, nil); err != nil {
		t.Fatal(err)
	}

	// A major pass settles the hole and promotes, and a minor pass runs.
	var stats block_gc.Stats
	if err := v.SweepUnreachedBlocks(ctx, rg, &stats); err != nil {
		t.Fatal(err)
	}
	if stats.AbsentBlocks != 1 || stats.UnknownBlocks != 0 || stats.BlocksSwept != 0 {
		t.Fatalf("major pass stats=%+v", stats)
	}
	var minor block_gc.Stats
	if err := v.SweepYoungBlocks(ctx, rg, &minor); err != nil {
		t.Fatal(err)
	}
	if minor.MinorPasses != 1 {
		t.Fatalf("minor pass did not run: stats=%+v", minor)
	}
	assertPublishedBlock(t, v, parent, true)
}

// TestMinorPassStopsOnUnknown checks that a minor pass that stops on a block
// without a child list withdraws the young log's readiness, so later minor
// passes do not run, also after a reopen, until a major pass promotes.
func TestMinorPassStopsOnUnknown(t *testing.T) {
	// Give up waiting for the list quickly.
	saved := awaitLimit
	awaitLimit = 100 * time.Millisecond
	t.Cleanup(func() { awaitLimit = saved })

	// Publish a parent that names a block stored without a child list, and
	// release a block so the minor pass has one to sweep.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	rg := v.GetRefGraph()
	parent := publicationFor(t, "parent", "", "one")
	if err := v.PublishAtomic(ctx, parent); err != nil {
		t.Fatal(err)
	}
	released := publicationFor(t, "released", "", "one")
	if err := v.PublishAtomic(ctx, released); err != nil {
		t.Fatal(err)
	}
	orphanPublication(t, v, released)
	unknown, _ := holeEntries(t)
	if _, _, err := v.PutBlock(ctx, unknown.Data, &block.PutOpts{ForceBlockRef: unknown.Ref}); err != nil {
		t.Fatal(err)
	}
	edge := block_gc.RefEdge{Subject: block_gc.BlockIRI(parent.Entries[0].Ref), Object: block_gc.BlockIRI(unknown.Ref)}
	if err := rg.ApplyRefBatch(ctx, []block_gc.RefEdge{edge}, nil); err != nil {
		t.Fatal(err)
	}
	minorPass := func() block_gc.Stats {
		t.Helper()
		var stats block_gc.Stats
		if err := v.SweepYoungBlocks(ctx, rg, &stats); err != nil {
			t.Fatal(err)
		}
		return stats
	}

	// The first minor pass stops on the unknown block and sweeps nothing.
	if stats := minorPass(); stats.MinorPasses != 1 || stats.UnknownBlocks != 1 || stats.BlocksSwept != 0 {
		t.Fatalf("first minor pass stats=%+v", stats)
	}

	// Later minor passes do not run, and the store records it.
	if stats := minorPass(); stats.MinorPasses != 0 {
		t.Fatalf("minor pass after a stop ran: stats=%+v", stats)
	}
	var ready bool
	err := v.refGraph.(*transactionRefGraph).read(ctx, func(g *block_gc.RefGraph) (err error) {
		ready, err = g.YoungReady(ctx)
		return err
	})
	if err != nil || ready {
		t.Fatalf("young ready after a stop: ready=%v err=%v", ready, err)
	}

	// Give the block its list; a major pass sweeps and promotes, and minor
	// passes run again.
	gs := block_gc.NewGCStoreOps(v, rg)
	if _, _, err := gs.PutBlock(ctx, unknown.Data, &block.PutOpts{ForceBlockRef: unknown.Ref, RefsKnown: true}); err != nil {
		t.Fatal(err)
	}
	if err := gs.FlushPending(ctx); err != nil {
		t.Fatal(err)
	}
	var stats block_gc.Stats
	if err := v.SweepUnreachedBlocks(ctx, rg, &stats); err != nil {
		t.Fatal(err)
	}
	if stats.UnknownBlocks != 0 || stats.BlocksSwept != 1 {
		t.Fatalf("major pass stats=%+v", stats)
	}
	if stats := minorPass(); stats.MinorPasses != 1 {
		t.Fatalf("minor pass after promotion did not run: stats=%+v", stats)
	}
	assertPublishedBlock(t, v, parent, true)
	assertPublishedBlock(t, v, released, false)
}
