package kvtx

import (
	"fmt"
	"io"
	"slices"
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
	// Open Volumes over one store, with helpers for each check.
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
	wantCounts := func(want block_gc.CountState) {
		t.Helper()
		state, err := openRefGraph(store).GetCountState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if state != want {
			t.Fatalf("count state = %+v, want %+v", state, want)
		}
	}

	// A store born empty is minor-ready and counted: a minor pass sweeps a
	// released block and keeps a published one, and the counts survive the
	// publications, the release and the sweep.
	v := open()
	live := publicationFor(t, "live", "", "one")
	if err := v.PublishAtomic(ctx, live); err != nil {
		t.Fatal(err)
	}
	first := publicationFor(t, "first", "", "one")
	if err := v.PublishAtomic(ctx, first); err != nil {
		t.Fatal(err)
	}

	// Release one and sweep it.
	orphanPublication(t, v, first)
	sweepYoung(v)
	assertPublishedBlock(t, v, first, false)
	assertPublishedBlock(t, v, live, true)
	complete := block_gc.CountState{Phase: block_gc.CountsComplete}
	wantCounts(complete)
	closeVolume(v)

	// Stand in for a store of format version 4 with blocks: drop the ready
	// record and the counts, kinds 'm', 's', 'k', 'q' and 'h' in the reference
	// graph, and rewind the version.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	var counts [][]byte
	for _, kind := range []byte{'m', 's', 'k', 'q', 'h'} {
		prefix := append(slices.Clone(refGraphPrefix), kind)
		err := tx.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
			counts = append(counts, slices.Clone(key))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range counts {
		if err := tx.Delete(ctx, key); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Set(ctx, keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(4)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// The upgraded store is not minor-ready, and builds its counts beside a
	// publication: a minor pass sweeps nothing.
	v = open()
	wantCounts(block_gc.CountState{Phase: block_gc.CountsWalking})
	second := publicationFor(t, "second", "", "one")
	if err := v.PublishAtomic(ctx, second); err != nil {
		t.Fatal(err)
	}
	if state, err := v.BuildCounts(ctx, time.Minute); err != nil || state != complete {
		t.Fatalf("build counts = %+v, %v", state, err)
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

	// Release and sweep it: the counts stay complete.
	orphanPublication(t, v, third)
	sweepYoung(v)
	assertPublishedBlock(t, v, third, false)
	assertPublishedBlock(t, v, live, true)
	wantCounts(complete)

	// The audit agrees with the counts.
	audit, err := v.AuditCounts(ctx, 0, 1)
	if err != nil || audit.MismatchCount != 0 || audit.Blocks == 0 {
		t.Fatalf("count audit = %+v, %v", audit, err)
	}

	// Lose a count record.
	tx, err = store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	var lost []byte
	err = tx.ScanPrefixKeys(ctx, append(slices.Clone(refGraphPrefix), 'k'), func(key []byte) error {
		lost = slices.Clone(key)
		return io.EOF
	})
	if err != io.EOF || tx.Delete(ctx, lost) != nil || tx.Commit(ctx) != nil {
		t.Fatalf("delete a count record: %v", err)
	}

	// The audit finds it and puts the counts in doubt.
	audit, err = v.AuditCounts(ctx, 0, 1)
	if err != nil || audit.MismatchCount == 0 {
		t.Fatalf("count audit after a lost record = %+v, %v", audit, err)
	}
	state, err := openRefGraph(store).GetCountState(ctx)
	if err != nil || state.Phase != block_gc.CountsDoubt {
		t.Fatalf("count state after the audit = %+v, %v", state, err)
	}
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

// TestMinorPassKeepsFilledHoleAWriteNamed checks that minor passes keep a hole
// that a write named during a major pass, which settled it with no namer to
// log, once a later write stores it: the write that named it logged the
// parent's changed list in the young log, past the bound the pass promotes.
func TestMinorPassKeepsFilledHoleAWriteNamed(t *testing.T) {
	for _, w := range holeWriters {
		t.Run(w.name, func(t *testing.T) {
			// Publish and release a block.
			v, _ := newPublicationTestVolume(t)
			ctx := t.Context()
			rg := v.GetRefGraph()
			released := publicationFor(t, "released", "", "one")
			if err := v.PublishAtomic(ctx, released); err != nil {
				t.Fatal(err)
			}
			orphanPublication(t, v, released)

			// Publish a parent, and pick a hole and its child.
			parent := publicationFor(t, "parent", "", "one")
			if err := v.PublishAtomic(ctx, parent); err != nil {
				t.Fatal(err)
			}
			hole, child := holeEntries(t)

			// A major pass in which a write names the hole after the mark
			// settles it, sweeps the released block and promotes.
			var stats block_gc.Stats
			err := v.runBlockPass(ctx, rg, false, func(actual *transactionRefGraph) error {
				// Take the promote bound before the pass begins.
				var bound uint64
				err := actual.read(ctx, func(rg *block_gc.RefGraph) (err error) {
					bound, err = rg.YoungPosition(ctx)
					return err
				})
				if err != nil {
					return err
				}

				// Run the pass body while the pass is open.
				return v.inBlockPass(ctx, actual, func() error {
					// Mark the blocks the roots reach.
					mark, err := actual.MarkBlocksScan(ctx)
					if err != nil {
						return err
					}

					// A write names the hole from the parent after the mark.
					edge := block_gc.RefEdge{Subject: block_gc.BlockIRI(parent.Entries[0].Ref), Object: block_gc.BlockIRI(hole.Ref)}
					if err := rg.ApplyRefBatch(ctx, []block_gc.RefEdge{edge}, nil); err != nil {
						return err
					}

					// Sweep, which must settle the hole, and promote.
					complete, err := v.sweepUnreached(ctx, mark, &stats)
					if err != nil || !complete {
						t.Fatalf("complete=%v err=%v stats=%+v", complete, err, stats)
					}
					return v.promoteYoung(ctx, mark, bound)
				})
			})
			if err != nil {
				t.Fatal(err)
			}

			// The pass settled the hole and swept the released block.
			if stats.AbsentBlocks != 1 || stats.UnknownBlocks != 0 || stats.BlocksSwept != 1 {
				t.Fatalf("major pass stats=%+v", stats)
			}
			assertPublishedBlock(t, v, released, false)

			// Minor passes run before and after a write stores the hole,
			// and keep it and its child once stored.
			minorPass := func(step string) {
				// Run one minor pass.
				t.Helper()
				var minor block_gc.Stats
				if err := v.SweepYoungBlocks(ctx, rg, &minor); err != nil {
					t.Fatal(err)
				}
				if minor.MinorPasses != 1 {
					t.Fatalf("%s: minor pass did not run: stats=%+v", step, minor)
				}

				// The hole and its child survive once stored, and the
				// parent stays published.
				for _, e := range []*block.PutBatchEntry{hole, child} {
					if _, found, err := v.GetBlock(ctx, e.Ref); err != nil {
						t.Fatal(err)
					} else if !found && step != "before fill" {
						t.Fatalf("%s: block a live parent names swept: swept=%d", step, minor.BlocksSwept)
					}
				}
				assertPublishedBlock(t, v, parent, true)
			}

			// A minor pass runs before the fill, then a write stores the hole
			// and its child, and two minor passes keep them.
			minorPass("before fill")
			w.write(t, v, rg, child, hole)
			minorPass("after fill")
			minorPass("after fill")
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

	// Publish a parent, and release a block so the minor pass has one to
	// sweep.
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

	// Store a block below the graph, without its list, and name it from the
	// parent.
	unknown, _ := holeEntries(t)
	if _, _, err := v.Store.PutBlock(ctx, unknown.Data, &block.PutOpts{ForceBlockRef: unknown.Ref}); err != nil {
		t.Fatal(err)
	}
	edge := block_gc.RefEdge{Subject: block_gc.BlockIRI(parent.Entries[0].Ref), Object: block_gc.BlockIRI(unknown.Ref)}
	if err := rg.ApplyRefBatch(ctx, []block_gc.RefEdge{edge}, nil); err != nil {
		t.Fatal(err)
	}

	// Run a minor pass and return its stats.
	minorPass := func() block_gc.Stats {
		t.Helper()
		var stats block_gc.Stats
		if err := v.SweepYoungBlocks(ctx, rg, &stats); err != nil {
			t.Fatal(err)
		}
		return stats
	}

	// The first minor pass stops on the unknown block, sweeps nothing and
	// reports the stop.
	if stats := minorPass(); stats.MinorPasses != 1 || stats.UnknownBlocks != 1 || stats.BlocksSwept != 0 || stats.MinorStopped == "" {
		t.Fatalf("first minor pass stats=%+v", stats)
	}

	// Later minor passes do not run or report the stop again, and the store
	// records it.
	if stats := minorPass(); stats.MinorPasses != 0 || stats.MinorStopped != "" {
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

	// Give the block its list.
	gs := block_gc.NewGCStoreOps(v, rg)
	if _, _, err := gs.PutBlock(ctx, unknown.Data, &block.PutOpts{ForceBlockRef: unknown.Ref, RefsKnown: true}); err != nil {
		t.Fatal(err)
	}
	if err := gs.FlushPending(ctx); err != nil {
		t.Fatal(err)
	}

	// A major pass sweeps and promotes, and minor passes run again.
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

// TestMinorPassKeepsFilledHoleARevivedParentNames checks that minor passes
// keep a filled hole whose only live namer is an old parent a write revived
// during the major pass that settled it. The same write adopts the hole
// through a new parent, so the catch-up reaches the hole with no namer before
// it walks the revived parent's list, and the new parent is swept later.
func TestMinorPassKeepsFilledHoleARevivedParentNames(t *testing.T) {
	for _, w := range holeWriters {
		t.Run(w.name, func(t *testing.T) {
			// Pick a hole and an old parent whose list names it.
			v, _ := newPublicationTestVolume(t)
			ctx := t.Context()
			rg := v.GetRefGraph()
			hole, child := holeEntries(t)
			old := publicationFor(t, "old-parent", "", "three").Entries[0]
			old.Refs, old.RefsKnown = []*block.BlockRef{hole.Ref}, true

			// Store the old parent as an unreached cache fill.
			gs := block_gc.NewGCStoreOps(v, rg)
			opts := &block.PutOpts{ForceBlockRef: old.Ref, Refs: old.Refs, CacheFill: true, RefsKnown: true}
			if _, _, err := gs.PutBlock(ctx, old.Data, opts); err != nil {
				t.Fatal(err)
			}
			if err := gs.FlushPending(ctx); err != nil {
				t.Fatal(err)
			}

			// A major pass in which one write revives the old parent and
			// names the hole from a new parent, then settles and promotes.
			newParent := publicationFor(t, "new-parent", "", "four").Entries[0]
			var stats block_gc.Stats
			err := v.runBlockPass(ctx, rg, false, func(actual *transactionRefGraph) error {
				// Take the promote bound before the pass begins.
				var bound uint64
				err := actual.read(ctx, func(rg *block_gc.RefGraph) (err error) {
					bound, err = rg.YoungPosition(ctx)
					return err
				})
				if err != nil {
					return err
				}

				// Run the pass body while the pass is open.
				return v.inBlockPass(ctx, actual, func() error {
					// Mark the blocks the roots reach.
					mark, err := actual.MarkBlocksScan(ctx)
					if err != nil {
						return err
					}

					// One write revives the old parent and names the hole
					// from a new parent.
					edges := []block_gc.RefEdge{
						{Subject: block_gc.NodeGCRoot, Object: block_gc.BlockIRI(old.Ref)},
						{Subject: block_gc.BlockIRI(newParent.Ref), Object: block_gc.BlockIRI(hole.Ref)},
					}
					if err := rg.ApplyRefBatch(ctx, edges, nil); err != nil {
						return err
					}

					// Sweep, which settles the hole, and promote.
					complete, err := v.sweepUnreached(ctx, mark, &stats)
					if err != nil || !complete {
						t.Fatalf("complete=%v err=%v stats=%+v", complete, err, stats)
					}
					return v.promoteYoung(ctx, mark, bound)
				})
			})
			if err != nil {
				t.Fatal(err)
			}

			// The pass settled the hole, so it promoted.
			if stats.AbsentBlocks != 1 || stats.UnknownBlocks != 0 {
				t.Fatalf("major pass stats=%+v", stats)
			}

			// Minor passes run before and after a write stores the hole.
			// The new parent is unreached and goes; the old parent stays
			// live through the root and names the hole.
			minorPass := func(step string) {
				// Run one minor pass.
				t.Helper()
				var minor block_gc.Stats
				if err := v.SweepYoungBlocks(ctx, rg, &minor); err != nil {
					t.Fatal(err)
				}
				if minor.MinorPasses != 1 {
					t.Fatalf("%s: minor pass did not run: stats=%+v", step, minor)
				}

				// The old parent stays, and the hole and its child survive
				// once stored.
				if _, found, err := v.GetBlock(ctx, old.Ref); err != nil || !found {
					t.Fatalf("%s: live old parent swept: found=%v err=%v", step, found, err)
				}
				for _, e := range []*block.PutBatchEntry{hole, child} {
					if _, found, err := v.GetBlock(ctx, e.Ref); err != nil {
						t.Fatal(err)
					} else if !found && step != "before fill" {
						t.Fatalf("%s: block a live parent names swept: stats=%+v", step, minor)
					}
				}
			}
			minorPass("before fill")
			w.write(t, v, rg, child, hole)
			minorPass("after fill")
			minorPass("after fill")
		})
	}
}

// TestMinorPassKeepsFilledHoleARevivalInALaterChunkNames checks that a major
// pass whose sweep spans several transactions logs the old parent that a
// write revives after the first transaction commits, and that minor passes
// keep the hole it names once filled. In one case the same write names the
// hole from a new parent, so the next rescue reaches both; in the other an
// earlier write named the hole, and the first transaction settled it with no
// namer before the old parent's list was read.
func TestMinorPassKeepsFilledHoleARevivalInALaterChunkNames(t *testing.T) {
	for _, settledFirst := range []bool{false, true} {
		for _, w := range holeWriters {
			name := fmt.Sprintf("settled-first=%v/%s", settledFirst, w.name)
			t.Run(name, func(t *testing.T) {
				testRevivalInALaterChunk(t, w, settledFirst)
			})
		}
	}
}

// testRevivalInALaterChunk runs one case of
// TestMinorPassKeepsFilledHoleARevivalInALaterChunkNames.
func testRevivalInALaterChunk(t *testing.T, w holeWriter, settledFirst bool) {
	// Release two blocks for the sweep's first chunks.
	v, raw := newPublicationTestVolume(t)
	ctx := t.Context()
	rg := v.GetRefGraph()
	var fillers []*block.AtomicPublication
	for i := range 2 {
		p := publicationFor(t, fmt.Sprintf("filler-%d", i), "", "one")
		if err := v.PublishAtomic(ctx, p); err != nil {
			t.Fatal(err)
		}
		orphanPublication(t, v, p)
		fillers = append(fillers, p)
	}

	// Store an unreached old parent whose list names a hole.
	hole, child := holeEntries(t)
	old := publicationFor(t, "old-parent", "", "three").Entries[0]
	old.Refs, old.RefsKnown = []*block.BlockRef{hole.Ref}, true
	gs := block_gc.NewGCStoreOps(v, rg)
	opts := &block.PutOpts{ForceBlockRef: old.Ref, Refs: old.Refs, CacheFill: true, RefsKnown: true}
	if _, _, err := gs.PutBlock(ctx, old.Data, opts); err != nil {
		t.Fatal(err)
	}
	if err := gs.FlushPending(ctx); err != nil {
		t.Fatal(err)
	}

	// Name the edges of the revival: the root to the old parent, and a new
	// parent to the hole.
	oldNode := block_gc.BlockIRI(old.Ref)
	revive := block_gc.RefEdge{Subject: block_gc.NodeGCRoot, Object: oldNode}
	newParent := publicationFor(t, "new-parent", "", "four").Entries[0]
	adopt := block_gc.RefEdge{Subject: block_gc.BlockIRI(newParent.Ref), Object: block_gc.BlockIRI(hole.Ref)}

	// Run a major pass whose sweep commits one block per transaction.
	var stats block_gc.Stats
	err := v.runBlockPass(ctx, rg, false, func(actual *transactionRefGraph) error {
		// Take the promote bound before the pass begins.
		var bound uint64
		err := actual.read(ctx, func(rg *block_gc.RefGraph) (err error) {
			bound, err = rg.YoungPosition(ctx)
			return err
		})
		if err != nil {
			return err
		}

		// Run the pass body while the pass is open.
		return v.inBlockPass(ctx, actual, func() error {
			// Mark, and sweep the old parent last.
			mark, err := actual.MarkBlocksScan(ctx)
			if err != nil {
				return err
			}
			at := slices.Index(mark.Unreached, oldNode)
			if at < 0 || len(mark.Unreached) != 3 {
				t.Fatalf("unreached %d blocks, old parent at %d", len(mark.Unreached), at)
			}
			mark.Unreached = append(slices.Delete(mark.Unreached, at, at+1), oldNode)

			// Name the hole before the sweep when it settles first.
			if settledFirst {
				if err := rg.ApplyRefBatch(ctx, []block_gc.RefEdge{adopt}, nil); err != nil {
					return err
				}
			}

			// Make every sweep transaction yield after one block, and hold
			// the first at the gate.
			raw.mu.Lock()
			raw.writersWaiting = true
			raw.mu.Unlock()
			first := raw.arm(t)

			// Sweep in the background.
			type result struct {
				complete bool
				err      error
			}
			done := make(chan result, 1)
			go func() {
				complete, err := v.sweepUnreached(ctx, mark, &stats)
				done <- result{complete, err}
			}()

			// Let the first transaction commit, and hold the second.
			<-first.started
			second := raw.arm(t)
			first.unblock()
			<-second.started

			// The first transaction swept one released block and left the
			// old parent.
			var swept int
			for _, p := range fillers {
				if _, found, err := v.GetBlock(ctx, p.Entries[0].Ref); err != nil {
					return err
				} else if !found {
					swept++
				}
			}
			if _, found, err := v.GetBlock(ctx, old.Ref); err != nil || !found || swept != 1 {
				t.Fatalf("after the first transaction: swept %d, old parent found=%v err=%v", swept, found, err)
			}

			// Revive the old parent while the second waits, naming the hole
			// from the new parent in the same write unless it settled first.
			edges := []block_gc.RefEdge{revive}
			if !settledFirst {
				edges = append(edges, adopt)
			}
			if err := rg.ApplyRefBatch(ctx, edges, nil); err != nil {
				return err
			}
			second.unblock()

			// The sweep finishes and the pass promotes.
			r := <-done
			if r.err != nil || !r.complete {
				t.Fatalf("complete=%v err=%v stats=%+v", r.complete, r.err, stats)
			}
			raw.mu.Lock()
			raw.writersWaiting = false
			raw.mu.Unlock()
			return v.promoteYoung(ctx, mark, bound)
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	// The pass settled the hole and swept both released blocks.
	if stats.AbsentBlocks != 1 || stats.UnknownBlocks != 0 || stats.BlocksSwept != 2 {
		t.Fatalf("major pass stats=%+v", stats)
	}
	for _, p := range fillers {
		assertPublishedBlock(t, v, p, false)
	}

	// Minor passes run before and after a write stores the hole.
	minorPass := func(step string) {
		// Run one minor pass.
		t.Helper()
		var minor block_gc.Stats
		if err := v.SweepYoungBlocks(ctx, rg, &minor); err != nil {
			t.Fatal(err)
		}
		if minor.MinorPasses != 1 {
			t.Fatalf("%s: minor pass did not run: stats=%+v", step, minor)
		}

		// The old parent stays, and the hole and its child survive once
		// stored.
		if _, found, err := v.GetBlock(ctx, old.Ref); err != nil || !found {
			t.Fatalf("%s: live old parent swept: found=%v err=%v", step, found, err)
		}
		for _, e := range []*block.PutBatchEntry{hole, child} {
			if _, found, err := v.GetBlock(ctx, e.Ref); err != nil {
				t.Fatal(err)
			} else if !found && step != "before fill" {
				t.Fatalf("%s: block a live parent names swept: stats=%+v", step, minor)
			}
		}
	}
	minorPass("before fill")
	w.write(t, v, rg, child, hole)
	minorPass("after fill")
	minorPass("after fill")
}
