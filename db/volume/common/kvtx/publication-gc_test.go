package kvtx

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

func orphanPublication(t *testing.T, v *Volume, p *block.AtomicPublication) string {
	// Remove the publication bucket edge to orphan its block.
	t.Helper()
	node := block_gc.BlockIRI(p.Entries[0].Ref)
	if err := v.GetRefGraph().ApplyRefBatch(t.Context(), nil, []block_gc.RefEdge{{Subject: block_gc.BucketIRI(p.BucketID), Object: node}}); err != nil {
		t.Fatal(err)
	}

	// Verify a mark no longer reaches the publication block.
	mark := markBlocks(t, v)
	if !slices.Contains(mark.Unreached, node) {
		t.Fatalf("block %q reached: %v", node, mark.Unreached)
	}
	return node
}

// markBlocks marks the blocks of the Volume's graph.
func markBlocks(t *testing.T, v *Volume) *block_gc.BlockMark {
	t.Helper()
	mark, err := v.refGraph.(*transactionRefGraph).MarkBlocks(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return mark
}

// TestPublicationSweepRescuesAdoptedBlock checks that a block a publication
// adopts after the mark's snapshot survives the sweep of an open pass.
func TestPublicationSweepRescuesAdoptedBlock(t *testing.T) {
	// Publish a block and release its bucket ownership.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	p := publicationFor(t, "sweep", "", "one")
	if err := v.PublishAtomic(ctx, p); err != nil {
		t.Fatal(err)
	}
	node := orphanPublication(t, v, p)

	// Open a pass and mark the block unreached.
	rg := v.refGraph.(*transactionRefGraph)
	if err := rg.write(ctx, func(rg *block_gc.RefGraph) error { return rg.BeginBlockPass(ctx) }); err != nil {
		t.Fatal(err)
	}
	mark := markBlocks(t, v)
	if !slices.Contains(mark.Unreached, node) {
		t.Fatalf("block %q reached: %v", node, mark.Unreached)
	}

	// Publication adopts the same bytes before the sweep begins.
	p.Heads = publicationFor(t, "sweep", "one", "two").Heads
	if err := v.PublishAtomic(ctx, p); err != nil {
		t.Fatal(err)
	}

	// The sweep rescues the block and keeps it and its new head.
	var stats block_gc.Stats
	if _, err := v.sweepUnreached(ctx, mark, &stats); err != nil {
		t.Fatal(err)
	}
	if stats.RescuedNodes == 0 || stats.BlocksSwept != 0 {
		t.Fatalf("rescue stats=%+v", stats)
	}
	assertPublishedBlock(t, v, p, true)
	assertHead(t, v, "sweep", "two")

	// Ending the pass drops its marker and log, so a later pass sweeps the
	// block once it is released again.
	if err := rg.write(ctx, func(rg *block_gc.RefGraph) error { return rg.EndBlockPass(ctx) }); err != nil {
		t.Fatal(err)
	}
	orphanPublication(t, v, p)
	collected, err := block_gc.NewCollector(rg, v, nil).Collect(ctx)
	if err != nil || collected.BlocksSwept != 1 {
		t.Fatalf("later pass stats=%+v err=%v", collected, err)
	}
	assertPublishedBlock(t, v, p, false)
}

// TestPublicationSweepUnknownBlocks checks that a sweep removes nothing while
// a block of unknown references, which may name any unreached block, remains:
// it stops when the mark could not list them, and otherwise waits for a
// commit that records their lists.
func TestPublicationSweepUnknownBlocks(t *testing.T) {
	// Publish a block, release it, and mark it unreached in an open pass.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	p := publicationFor(t, "unknown", "", "one")
	if err := v.PublishAtomic(ctx, p); err != nil {
		t.Fatal(err)
	}
	orphanPublication(t, v, p)
	rg := v.refGraph.(*transactionRefGraph)
	if err := rg.write(ctx, func(rg *block_gc.RefGraph) error { return rg.BeginBlockPass(ctx) }); err != nil {
		t.Fatal(err)
	}
	mark := markBlocks(t, v)

	// Stand in for a mark that counted a block without a list it could
	// not name: the sweep keeps the block and reports the unknown block.
	mark.Unknown, mark.UnknownSample = 1, []string{"untracked"}
	var stats block_gc.Stats
	if _, err := v.sweepUnreached(ctx, mark, &stats); err != nil {
		t.Fatal(err)
	}
	if stats.BlocksSwept != 0 || stats.UnknownBlocks != 1 || !slices.Equal(stats.UnknownSample, []string{"untracked"}) {
		t.Fatalf("stats=%+v", stats)
	}
	if _, found, err := v.GetBlock(ctx, p.Entries[0].Ref); err != nil || !found {
		t.Fatalf("block found=%v err=%v", found, err)
	}

	// A bucket adopts a parent block whose staging flush, which records
	// that it references the released block, has not committed yet.
	mark.Unknown, mark.UnknownSample = 0, nil
	parent := block_gc.BlockIRI(publicationFor(t, "parent", "", "two").Entries[0].Ref)
	child := block_gc.BlockIRI(p.Entries[0].Ref)
	if err := rg.AddRef(ctx, "bucket", parent); err != nil {
		t.Fatal(err)
	}

	// The flush commits after the sweep's first rescue consumes the log,
	// so the sweep waits for it.
	rescued := v.nextCommit()
	flushed := make(chan error, 1)
	go func() {
		<-rescued
		flushed <- rg.ApplyRefBatch(ctx, []block_gc.RefEdge{
			{Subject: block_gc.NodeUnreferenced, Object: parent},
			{Subject: parent, Object: child},
		}, nil)
	}()
	stats = block_gc.Stats{UnknownBlocks: 1}
	if _, err := v.sweepUnreached(ctx, mark, &stats); err != nil {
		t.Fatal(err)
	}
	if err := <-flushed; err != nil {
		t.Fatal(err)
	}

	// The rescue after the flush reaches the block through the parent, and
	// the sweep reports no block unknown, whatever the mark counted.
	if stats.BlocksSwept != 0 || stats.UnknownBlocks != 0 || mark.Unknown != 0 || !mark.Live.Has(child) {
		t.Fatalf("stats=%+v unknown=%d child live=%v", stats, mark.Unknown, mark.Live.Has(child))
	}
	if _, found, err := v.GetBlock(ctx, p.Entries[0].Ref); err != nil || !found {
		t.Fatalf("block found=%v err=%v", found, err)
	}
}

// TestPublicationSweepPastAbsentBlock checks that a block pass sweeps past a
// block a parent's list names that has no list and that the store does not
// hold, such as a hole a copy recorded, since it names nothing a reader can
// reach.
func TestPublicationSweepPastAbsentBlock(t *testing.T) {
	// Publish a block and release it, and publish a parent.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	p := publicationFor(t, "absent", "", "one")
	if err := v.PublishAtomic(ctx, p); err != nil {
		t.Fatal(err)
	}
	orphanPublication(t, v, p)
	parent := publicationFor(t, "parent", "", "two")
	if err := v.PublishAtomic(ctx, parent); err != nil {
		t.Fatal(err)
	}

	// The parent's list names a block the store never held, which a mark
	// counts as unknown.
	hole := block_gc.BlockIRI(publicationFor(t, "hole", "", "three").Entries[0].Ref)
	rg := v.refGraph.(*transactionRefGraph)
	edge := block_gc.RefEdge{Subject: block_gc.BlockIRI(parent.Entries[0].Ref), Object: hole}
	if err := rg.ApplyRefBatch(ctx, []block_gc.RefEdge{edge}, nil); err != nil {
		t.Fatal(err)
	}
	if mark := markBlocks(t, v); mark.Unknown != 1 {
		t.Fatalf("unknown=%d", mark.Unknown)
	}

	// The pass settles the hole, sweeps the released block and keeps the
	// parent.
	stats, err := block_gc.NewCollector(rg, v, nil).Collect(ctx)
	if err != nil || stats.BlocksSwept != 1 || stats.AbsentBlocks != 1 || stats.UnknownBlocks != 0 {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
	assertPublishedBlock(t, v, p, false)
	assertPublishedBlock(t, v, parent, true)
}

// manyHoles is more holes than a mark lists at a time, block_gc's
// settleLimit, as a lazy replica's lists name.
const manyHoles = 5000

// nameHoles publishes a parent whose list names n blocks the store never held,
// and returns the parent and the holes.
func nameHoles(t *testing.T, v *Volume, n int) (*block.AtomicPublication, []block_gc.RefEdge) {
	// Publish the parent.
	t.Helper()
	parent := publicationFor(t, "parent", "", "two")
	if err := v.PublishAtomic(t.Context(), parent); err != nil {
		t.Fatal(err)
	}

	// Name the holes from the parent.
	edges := make([]block_gc.RefEdge, n)
	for i := range edges {
		hole := block_gc.BlockIRI(publicationFor(t, "hole", "", fmt.Sprint("hole-", i)).Entries[0].Ref)
		edges[i] = block_gc.RefEdge{Subject: block_gc.BlockIRI(parent.Entries[0].Ref), Object: hole}
	}
	if err := v.GetRefGraph().ApplyRefBatch(t.Context(), edges, nil); err != nil {
		t.Fatal(err)
	}
	return parent, edges
}

// TestPublicationSweepPastManyAbsentBlocks checks that a block pass sweeps past
// more holes than a mark lists at a time, as a replica that fetched a parent
// and few of its children holds.
func TestPublicationSweepPastManyAbsentBlocks(t *testing.T) {
	// Release a block, and name many holes from a parent.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	p := publicationFor(t, "absent", "", "one")
	if err := v.PublishAtomic(ctx, p); err != nil {
		t.Fatal(err)
	}
	orphanPublication(t, v, p)
	parent, _ := nameHoles(t, v, manyHoles)

	// The pass settles every hole, sweeps the released block and keeps the
	// parent.
	rg := v.refGraph.(*transactionRefGraph)
	stats, err := block_gc.NewCollector(rg, v, nil).Collect(ctx)
	if err != nil || stats.BlocksSwept != 1 || stats.AbsentBlocks != manyHoles || stats.UnknownBlocks != 0 {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
	assertPublishedBlock(t, v, p, false)
	assertPublishedBlock(t, v, parent, true)
}

// TestPublicationSweepStopsAtStoredBlockAmongHoles checks that a block the
// store holds without a child list still stops a pass when many holes sit
// beside it: it may name any unreached block.
func TestPublicationSweepStopsAtStoredBlockAmongHoles(t *testing.T) {
	// Release a block, and name many holes from a parent.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	p := publicationFor(t, "absent", "", "one")
	if err := v.PublishAtomic(ctx, p); err != nil {
		t.Fatal(err)
	}
	orphanPublication(t, v, p)
	parent, _ := nameHoles(t, v, manyHoles)

	// Store a block the parent names below the graph, without its list.
	raw := publicationFor(t, "raw", "", "three").Entries[0]
	if _, _, err := v.Store.PutBlock(ctx, raw.Data, &block.PutOpts{ForceBlockRef: raw.Ref}); err != nil {
		t.Fatal(err)
	}
	edge := block_gc.RefEdge{Subject: block_gc.BlockIRI(parent.Entries[0].Ref), Object: block_gc.BlockIRI(raw.Ref)}
	if err := v.GetRefGraph().ApplyRefBatch(ctx, []block_gc.RefEdge{edge}, nil); err != nil {
		t.Fatal(err)
	}

	// The mark settles the holes and counts the stored block, which stops a
	// sweep, as TestPublicationSweepUnknownBlocks checks.
	rg := v.refGraph.(*transactionRefGraph)
	if err := rg.write(ctx, func(rg *block_gc.RefGraph) error { return rg.BeginBlockPass(ctx) }); err != nil {
		t.Fatal(err)
	}
	mark, err := rg.MarkBlocksScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if mark.Absent != manyHoles || mark.Unknown != 1 || !slices.Equal(mark.UnknownSample, []string{block_gc.BlockIRI(raw.Ref)}) {
		t.Fatalf("absent=%d unknown=%d sample=%v", mark.Absent, mark.Unknown, mark.UnknownSample)
	}
}

// TestPublicationSweepKeepsBlockAHoleNamesOnceStored checks that a write that
// stores a hole after the mark settled it keeps the unreached block the hole
// names: the write logs the block it adopts and the sweep rescues it.
func TestPublicationSweepKeepsBlockAHoleNamesOnceStored(t *testing.T) {
	for _, w := range holeWriters {
		t.Run(w.name, func(t *testing.T) {
			// Release a block.
			v, _ := newPublicationTestVolume(t)
			ctx := t.Context()
			released := publicationFor(t, "released", "", "one")
			if err := v.PublishAtomic(ctx, released); err != nil {
				t.Fatal(err)
			}
			orphanPublication(t, v, released)

			// Name many holes from a parent, the first of which names
			// the released block once stored.
			parent, edges := nameHoles(t, v, manyHoles)
			hole := publicationFor(t, "hole", "", "hole-0").Entries[0]
			if block_gc.BlockIRI(hole.Ref) != edges[0].Object {
				t.Fatal("hole entry does not match the first hole")
			}
			hole.Refs, hole.RefsKnown = []*block.BlockRef{released.Entries[0].Ref}, true

			// Open a pass, mark, and settle the holes.
			rg := v.refGraph.(*transactionRefGraph)
			if err := rg.write(ctx, func(rg *block_gc.RefGraph) error { return rg.BeginBlockPass(ctx) }); err != nil {
				t.Fatal(err)
			}
			mark, err := rg.MarkBlocksScan(ctx)
			if err != nil {
				t.Fatal(err)
			}
			node := block_gc.BlockIRI(released.Entries[0].Ref)
			if mark.Absent != manyHoles || mark.Unknown != 0 || !slices.Contains(mark.Unreached, node) {
				t.Fatalf("absent=%d unknown=%d unreached=%v", mark.Absent, mark.Unknown, mark.Unreached)
			}

			// A write stores the hole before the sweep.
			child := released.Entries[0]
			child.RefsKnown = true
			w.write(t, v, rg, child, hole)

			// The sweep rescues the released block.
			var stats block_gc.Stats
			if _, err := v.sweepUnreached(ctx, mark, &stats); err != nil {
				t.Fatal(err)
			}
			if stats.BlocksSwept != 0 || !mark.Live.Has(node) {
				t.Fatalf("stats=%+v released live=%v", stats, mark.Live.Has(node))
			}
			for _, e := range []*block.PutBatchEntry{hole, child} {
				if _, found, err := v.GetBlock(ctx, e.Ref); err != nil || !found {
					t.Fatalf("block found=%v err=%v", found, err)
				}
			}
			assertPublishedBlock(t, v, parent, true)
		})
	}
}

// TestPublicationSweepPastHoleAWriteNames checks that a block pass sweeps past
// a hole that a write during the pass names, as a lazy replica's cache fill of
// a parent whose children it did not fetch does: the rescue reaches the hole
// through the adopt log, with no parent as its namer, and the store does not
// hold it.
func TestPublicationSweepPastHoleAWriteNames(t *testing.T) {
	// Give up waiting for a list quickly.
	saved := awaitLimit
	awaitLimit = 100 * time.Millisecond
	t.Cleanup(func() { awaitLimit = saved })

	// Release a block, and publish a parent.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	released := publicationFor(t, "released", "", "one")
	if err := v.PublishAtomic(ctx, released); err != nil {
		t.Fatal(err)
	}
	orphanPublication(t, v, released)
	parent := publicationFor(t, "parent", "", "two")
	if err := v.PublishAtomic(ctx, parent); err != nil {
		t.Fatal(err)
	}

	// Open a pass and mark.
	rg := v.refGraph.(*transactionRefGraph)
	if err := rg.write(ctx, func(rg *block_gc.RefGraph) error { return rg.BeginBlockPass(ctx) }); err != nil {
		t.Fatal(err)
	}
	mark, err := rg.MarkBlocksScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	node := block_gc.BlockIRI(released.Entries[0].Ref)
	if mark.Unknown != 0 || !slices.Contains(mark.Unreached, node) {
		t.Fatalf("unknown=%d unreached=%v", mark.Unknown, mark.Unreached)
	}

	// A write names a hole from the parent after the mark.
	hole, _ := holeEntries(t)
	edge := block_gc.RefEdge{Subject: block_gc.BlockIRI(parent.Entries[0].Ref), Object: block_gc.BlockIRI(hole.Ref)}
	if err := v.GetRefGraph().ApplyRefBatch(ctx, []block_gc.RefEdge{edge}, nil); err != nil {
		t.Fatal(err)
	}

	// The sweep settles the hole and sweeps the released block.
	var stats block_gc.Stats
	complete, err := v.sweepUnreached(ctx, mark, &stats)
	if err != nil {
		t.Fatal(err)
	}
	if !complete || stats.BlocksSwept != 1 || stats.UnknownBlocks != 0 || stats.AbsentBlocks != 1 {
		t.Fatalf("complete=%v stats=%+v", complete, stats)
	}
	assertPublishedBlock(t, v, released, false)
	assertPublishedBlock(t, v, parent, true)
}

// TestPublicationSweepStopsAtOwnedBlockAWriteNames checks that a block an
// owner adopts during the pass, before its staging flush records its list,
// stops the sweep while the store lacks it, even when a parent's list also
// names it: the write may still be storing it.
func TestPublicationSweepStopsAtOwnedBlockAWriteNames(t *testing.T) {
	// Give up waiting for a list quickly.
	saved := awaitLimit
	awaitLimit = 100 * time.Millisecond
	t.Cleanup(func() { awaitLimit = saved })

	// Publish and release a block, and publish a parent.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	released := publicationFor(t, "released", "", "one")
	if err := v.PublishAtomic(ctx, released); err != nil {
		t.Fatal(err)
	}
	orphanPublication(t, v, released)
	parent := publicationFor(t, "parent", "", "two")
	if err := v.PublishAtomic(ctx, parent); err != nil {
		t.Fatal(err)
	}

	// Open a pass and mark.
	rg := v.refGraph.(*transactionRefGraph)
	if err := rg.write(ctx, func(rg *block_gc.RefGraph) error { return rg.BeginBlockPass(ctx) }); err != nil {
		t.Fatal(err)
	}
	mark, err := rg.MarkBlocksScan(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// A bucket adopts a block the store does not hold yet, and a write
	// names it from the parent, before the block's staging flush.
	owned, _ := holeEntries(t)
	node := block_gc.BlockIRI(owned.Ref)
	if err := rg.AddRef(ctx, "bucket", node); err != nil {
		t.Fatal(err)
	}
	err = v.GetRefGraph().ApplyRefBatch(ctx, []block_gc.RefEdge{
		{Subject: block_gc.BlockIRI(parent.Entries[0].Ref), Object: node},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// The sweep keeps the released block and reports the owned block.
	var stats block_gc.Stats
	complete, err := v.sweepUnreached(ctx, mark, &stats)
	if err != nil {
		t.Fatal(err)
	}
	if complete || stats.BlocksSwept != 0 || stats.UnknownBlocks != 1 || stats.AbsentBlocks != 0 {
		t.Fatalf("complete=%v stats=%+v", complete, stats)
	}
	if _, found, err := v.GetBlock(ctx, released.Entries[0].Ref); err != nil || !found {
		t.Fatalf("released block found=%v err=%v", found, err)
	}
}

// TestPublicationSweepBatchMarksReleasedChildren checks that one sweep batch
// marks each child it leaves without an owner, leaves a child another owner
// keeps unmarked, leaves no marker on the nodes it removes, and drops the
// stale marker of a candidate another owner took.
func TestPublicationSweepBatchMarksReleasedChildren(t *testing.T) {
	// Mark two orphans: one shares a child with a kept node, both own another.
	// The child also holds a marker from before the orphans took it, and a
	// kept node took the marked node taken.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	rg := v.GetRefGraph()
	unref := block_gc.NodeUnreferenced
	err := rg.ApplyRefBatch(ctx, []block_gc.RefEdge{
		{Subject: block_gc.NodeGCRoot, Object: "keep"},
		{Subject: "keep", Object: "shared"},
		{Subject: unref, Object: "o1"},
		{Subject: unref, Object: "o2"},
		{Subject: "o1", Object: "child"},
		{Subject: "o1", Object: "shared"},
		{Subject: "o2", Object: "child"},
		{Subject: unref, Object: "child"},
		{Subject: unref, Object: "taken"},
		{Subject: "keep", Object: "taken"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Sweep both in one batch, with a duplicate candidate.
	removed, err := v.SweepUnreferenced(ctx, rg, []string{"o1", "child", "taken", "o2", "o1"})
	if err != nil || !slices.Equal(removed, []string{"o1", "o2"}) {
		t.Fatalf("sweep: removed=%v err=%v", removed, err)
	}

	// Only the released child keeps a marker.
	for node, want := range map[string][]string{
		"o1":     nil,
		"o2":     nil,
		"child":  {unref},
		"shared": {"keep"},
		"taken":  {"keep"},
	} {
		got, err := rg.GetIncomingRefs(ctx, node)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s owners = %v, want %v", node, got, want)
		}
	}
}

// TestPublicationCollectForStopsAtBudget checks that a budgeted collection
// reports the cascade it leaves and that the next call sweeps it.
func TestPublicationCollectForStopsAtBudget(t *testing.T) {
	// Mark an orphan that owns a child alone.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	rg := v.GetRefGraph()
	err := rg.ApplyRefBatch(ctx, []block_gc.RefEdge{
		{Subject: block_gc.NodeUnreferenced, Object: "parent"},
		{Subject: "parent", Object: "child"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// A spent budget still sweeps the listed candidates, then stops.
	collector := block_gc.NewCollector(rg, v, nil)
	stats, err := collector.CollectFor(ctx, time.Nanosecond)
	if err != nil || stats.NodesSwept != 1 || !stats.More {
		t.Fatalf("first collection stats=%+v err=%v", stats, err)
	}

	// The next collection sweeps the released child and finishes.
	stats, err = collector.CollectFor(ctx, time.Minute)
	if err != nil || stats.NodesSwept != 1 || stats.More {
		t.Fatalf("second collection stats=%+v err=%v", stats, err)
	}
}

// TestPublicationCollectorUsesAtomicSweep checks that the collector sweeps
// through the Volume: a block pass sweeps an unreached block, and minor passes
// between block passes sweep a released young block and keep the live ones,
// including a young block that an old block names through a later edge.
func TestPublicationCollectorUsesAtomicSweep(t *testing.T) {
	// Publish and orphan the collector test block.
	v, _ := newPublicationTestVolume(t)
	p := publicationFor(t, "collector", "", "one")
	if err := v.PublishAtomic(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	node := orphanPublication(t, v, p)

	// The collector sweeps the unreached block through the Volume.
	stats, err := block_gc.NewCollector(v.GetRefGraph(), v, nil).Collect(t.Context())
	if err != nil || stats.BlockPasses != 1 || stats.NodesSwept != 1 || stats.RemoveBlockCount != 1 || stats.AtomicSweepCount != 1 {
		t.Fatalf("orphan stats=%+v err=%v", stats, err)
	}
	assertPublishedBlock(t, v, p, false)

	// The block left no records, and a repeat pass sweeps nothing.
	incoming, err := v.GetRefGraph().GetIncomingRefs(t.Context(), node)
	if err != nil || len(incoming) != 0 {
		t.Fatalf("swept block owners=%v err=%v", incoming, err)
	}
	stats, err = block_gc.NewCollector(v.GetRefGraph(), v, nil).Collect(t.Context())
	if err != nil || stats.NodesSwept != 0 {
		t.Fatalf("repeat stats=%+v err=%v", stats, err)
	}

	// Between block passes, the collector runs minor passes, which the
	// completed passes above enabled.
	ctx := t.Context()
	rg := v.GetRefGraph()
	collector := block_gc.NewCollector(rg, v, nil)
	collector.SetMarkInterval(time.Hour, time.Now().Add(time.Hour))
	minor := func(swept int) {
		t.Helper()
		stats, err := collector.CollectFor(ctx, time.Minute)
		if err != nil || stats.MinorPasses != 1 || stats.BlockPasses != 0 || stats.BlocksSwept != swept {
			t.Fatalf("minor stats=%+v err=%v", stats, err)
		}
	}

	// A minor pass sweeps a released young block and keeps a live one.
	dead := publicationFor(t, "young-dead", "", "one")
	kept := publicationFor(t, "young-kept", "", "one")
	for _, p := range []*block.AtomicPublication{dead, kept} {
		if err := v.PublishAtomic(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	release := func(p *block.AtomicPublication) {
		t.Helper()
		edge := block_gc.RefEdge{Subject: block_gc.BucketIRI(p.BucketID), Object: block_gc.BlockIRI(p.Entries[0].Ref)}
		if err := rg.ApplyRefBatch(ctx, nil, []block_gc.RefEdge{edge}); err != nil {
			t.Fatal(err)
		}
	}
	release(dead)
	minor(1)
	assertPublishedBlock(t, v, dead, false)
	assertPublishedBlock(t, v, kept, true)

	// The kept block is now old. A young block it names only through a
	// later edge survives a minor pass, which seeds the children of each
	// old block whose list changed, and the next one too.
	child := publicationFor(t, "young-child", "", "one")
	if err := v.PublishAtomic(ctx, child); err != nil {
		t.Fatal(err)
	}
	parentNode, childNode := block_gc.BlockIRI(kept.Entries[0].Ref), block_gc.BlockIRI(child.Entries[0].Ref)
	if err := rg.ApplyRefBatch(ctx, []block_gc.RefEdge{{Subject: parentNode, Object: childNode}}, nil); err != nil {
		t.Fatal(err)
	}
	release(child)
	minor(0)
	minor(0)
	if _, found, err := v.GetBlock(ctx, child.Entries[0].Ref); err != nil || !found {
		t.Fatalf("child found=%v err=%v", found, err)
	}
}

// TestPublicationBlockPassSweepsPastBudget checks that a block pass sweeps
// every block its mark left unreached, in transactions that yield to waiting
// writers, after the collection's budget is spent.
func TestPublicationBlockPassSweepsPastBudget(t *testing.T) {
	// Orphan more blocks than one sweep chunk holds.
	v, raw := newPublicationTestVolume(t)
	var published []*block.AtomicPublication
	for i := range sweepChunkSize + 1 {
		p := publicationFor(t, fmt.Sprintf("budget-%d", i), "", "one")
		if err := v.PublishAtomic(t.Context(), p); err != nil {
			t.Fatal(err)
		}
		node := block_gc.BlockIRI(p.Entries[0].Ref)
		if err := v.GetRefGraph().ApplyRefBatch(t.Context(), nil, []block_gc.RefEdge{{Subject: block_gc.BucketIRI(p.BucketID), Object: node}}); err != nil {
			t.Fatal(err)
		}
		published = append(published, p)
	}

	// A writer always waits, so each sweep transaction takes one block.
	raw.mu.Lock()
	raw.writersWaiting = true
	before := raw.commits
	raw.mu.Unlock()

	// A spent budget still sweeps the whole pass, committing each block
	// alone.
	stats, err := block_gc.NewCollector(v.GetRefGraph(), v, nil).CollectFor(t.Context(), time.Nanosecond)
	if err != nil || stats.BlockPasses != 1 || stats.BlocksSwept != len(published) || stats.More {
		t.Fatalf("collection stats=%+v err=%v", stats, err)
	}
	raw.mu.Lock()
	count := raw.commits - before
	raw.mu.Unlock()
	if count < len(published) {
		t.Fatalf("pass used %d commits for %d blocks, want one per block", count, len(published))
	}
	for _, p := range published {
		assertPublishedBlock(t, v, p, false)
	}
}

// TestPublicationBlockPassMarkSetting checks that a block pass sweeps the same
// blocks whether it marks by scan, the default, or by the walk the setting
// selects, and that the scan leaves directMu released.
func TestPublicationBlockPassMarkSetting(t *testing.T) {
	for _, walk := range []bool{false, true} {
		t.Run(fmt.Sprintf("walk=%v", walk), func(t *testing.T) {
			// Publish two blocks and release the first's ownership.
			v, _ := newPublicationTestVolume(t)
			v.gcMarkWalk = walk
			released := publicationFor(t, "released", "", "one")
			kept := publicationFor(t, "kept", "", "two")
			for _, p := range []*block.AtomicPublication{released, kept} {
				if err := v.PublishAtomic(t.Context(), p); err != nil {
					t.Fatal(err)
				}
			}
			orphanPublication(t, v, released)

			// A pass sweeps only the released block.
			stats, err := block_gc.NewCollector(v.GetRefGraph(), v, nil).Collect(t.Context())
			if err != nil || stats.BlocksSwept != 1 || (stats.MarkScans != 0) == walk {
				t.Fatalf("collection stats=%+v err=%v", stats, err)
			}
			assertPublishedBlock(t, v, released, false)
			assertPublishedBlock(t, v, kept, true)

			// No mark transaction still holds directMu.
			if !v.directMu.TryLock() {
				t.Fatal("directMu is held after the pass")
			}
			v.directMu.Unlock()
		})
	}
}

func TestPublicationCollectorSweepsCandidatesInOneCommit(t *testing.T) {
	// Publish and orphan a batch of collector candidates.
	v, raw := newPublicationTestVolume(t)
	var published []*block.AtomicPublication
	for i := range 5 {
		p := publicationFor(t, fmt.Sprintf("batch-%d", i), "", "one")
		if err := v.PublishAtomic(t.Context(), p); err != nil {
			t.Fatal(err)
		}
		orphanPublication(t, v, p)
		published = append(published, p)
	}

	// Record the physical commit count before collecting the candidates.
	raw.mu.Lock()
	before := raw.commits
	raw.mu.Unlock()

	// Collect the orphaned batch and verify every block is swept.
	stats, err := block_gc.NewCollector(v.GetRefGraph(), v, nil).Collect(t.Context())
	if err != nil || stats.NodesSwept != len(published) || stats.RemoveBlockCount != len(published) {
		t.Fatalf("collector stats=%+v err=%v", stats, err)
	}

	// Verify the collector sweeps the whole batch in one physical commit
	// between the commit that opens the pass and those that promote its mark
	// and close it.
	raw.mu.Lock()
	count := raw.commits - before
	raw.mu.Unlock()
	if count != 4 {
		t.Fatalf("pass used %d physical commits for %d candidates, want 4", count, len(published))
	}
	for _, p := range published {
		assertPublishedBlock(t, v, p, false)
	}
}

func TestPublicationSweepFailureRollsBackGraphAndBlock(t *testing.T) {
	// Publish, orphan and mark a block for the failed sweep.
	v, raw := newPublicationTestVolume(t)
	p := publicationFor(t, "sweep-failure", "", "one")
	if err := v.PublishAtomic(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	node := orphanPublication(t, v, p)
	mark := markBlocks(t, v)

	// Inject a physical commit failure into the sweep.
	failure := errors.New("sync failed")
	raw.mu.Lock()
	raw.errorCommit = failure
	raw.mu.Unlock()

	// Verify the failed sweep reports no deleted blocks.
	var stats block_gc.Stats
	_, err := v.sweepUnreached(t.Context(), mark, &stats)
	if stats.BlocksSwept != 0 || !errors.Is(err, failure) {
		t.Fatalf("failed sweep: stats=%+v err=%v", stats, err)
	}

	// Clear the commit fault before inspecting stored state.
	raw.mu.Lock()
	raw.errorCommit = nil
	raw.mu.Unlock()

	// Verify the failed sweep preserves block bytes and the child list.
	data, found, err := v.GetBlock(t.Context(), p.Entries[0].Ref)
	if err != nil || !found || !bytes.Equal(data, p.Entries[0].Data) {
		t.Fatal("failed sweep deleted bytes", err)
	}
	incoming, err := v.GetRefGraph().GetIncomingRefs(t.Context(), node)
	if err != nil || !slices.Contains(incoming, block_gc.NodeUnreferenced) {
		t.Fatal("failed sweep deleted child list", incoming, err)
	}

	// Verify a later pass removes the preserved block.
	collected, err := block_gc.NewCollector(v.GetRefGraph(), v, nil).Collect(t.Context())
	if err != nil || collected.BlocksSwept != 1 {
		t.Fatalf("retry pass: stats=%+v err=%v", collected, err)
	}
	assertPublishedBlock(t, v, p, false)
}

func TestPublicationPreparationAtomicAndOversized(t *testing.T) {
	// Open a Volume for direct oversized block preparation.
	v, raw := newPublicationTestVolume(t)

	// Larger than the World staging cap: direct preparation is not admitted
	// into the bounded publication queue and cannot publish a head.
	data := bytes.Repeat([]byte("large-body"), (5<<20)/10)
	raw.mu.Lock()
	before := raw.commits
	raw.mu.Unlock()

	// Prepare the oversized block outside the publication queue.
	ref, existed, err := v.PrepareOwnedBlock(t.Context(), "large", data, leafOpts())
	if err != nil || existed {
		t.Fatalf("prepare: %v %v", existed, err)
	}

	// Verify direct preparation uses one physical commit.
	raw.mu.Lock()
	count := raw.commits - before
	raw.mu.Unlock()
	if count != 1 {
		t.Fatalf("preparation used %d physical commits", count)
	}

	// Verify preparation stores owned bytes without publishing a head.
	p := &block.AtomicPublication{BucketID: "large", Entries: []*block.PutBatchEntry{{Ref: ref, Data: data, RefsKnown: true}}}
	assertPublishedBlock(t, v, p, true)
	assertHead(t, v, "large", "")
	if stats := v.GetPublicationStats(); stats.Accepted != 0 {
		t.Fatalf("preparation was queued: %+v", stats)
	}

	// Verify preparing the same bytes reports the existing block.
	_, existed, err = v.PrepareOwnedBlock(t.Context(), "large", data, leafOpts())
	if err != nil || !existed {
		t.Fatalf("duplicate preparation: %v %v", existed, err)
	}

	// Inject a physical commit failure into block batch preparation.
	failure := errors.New("preparation commit failed")
	failed := publicationFor(t, "unpublished", "", "one")
	raw.mu.Lock()
	raw.errorCommit = failure
	raw.mu.Unlock()

	// Attempt the failing batch and clear the injected fault.
	_, err = v.PrepareOwnedBlockBatch(t.Context(), failed.BucketID, failed.Entries)
	raw.mu.Lock()
	raw.errorCommit = nil
	raw.mu.Unlock()

	// Verify failed preparation stores no owned block.
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	assertPublishedBlock(t, v, failed, false)
}

func TestPublicationDirectCloseAndGraphScope(t *testing.T) {
	// Verify atomic sweeping rejects another Volume reference graph.
	v, _ := newPublicationTestVolume(t)
	other, _ := newPublicationTestVolume(t)
	removed, err := v.SweepUnreferenced(t.Context(), other.GetRefGraph(), []string{"object:wrong-scope"})
	if len(removed) != 0 || !errors.Is(err, block_gc.ErrAtomicSweepUnsupported) {
		t.Fatalf("wrong scope: %v %v", removed, err)
	}

	// Close the Volume and verify it rejects direct preparation.
	if err := v.Close(); err != nil {
		t.Fatal(err)
	}
	_, _, err = v.PrepareOwnedBlock(t.Context(), "closed", []byte("data"), leafOpts())
	if !errors.Is(err, block.ErrPublicationClosed) {
		t.Fatal("preparation admitted after close", err)
	}
}

func TestPublicationGraphFailureRetainsWholeTransition(t *testing.T) {
	// Build a reference batch spanning multiple virtual graph slices.
	v, raw := newPublicationTestVolume(t)
	adds := make([]block_gc.RefEdge, 4100) // crosses RefGraph's virtual slice boundary
	for i := range adds {
		adds[i] = block_gc.RefEdge{Subject: "retry-owner", Object: fmt.Sprintf("object:retry-%d", i)}
	}

	// Inject a physical commit failure into the reference batch.
	failure := errors.New("outer graph commit failed")
	raw.mu.Lock()
	raw.errorCommit = failure
	raw.mu.Unlock()

	// Attempt the reference transition and clear the commit fault.
	err := v.GetRefGraph().ApplyRefBatch(t.Context(), adds, nil)
	raw.mu.Lock()
	raw.errorCommit = nil
	raw.mu.Unlock()

	// Verify the failure retains the whole reference transition for retry.
	ra, rr, ok := block_gc.RefBatchRemainder(err)
	if !errors.Is(err, failure) || !ok || len(ra) != len(adds) || len(rr) != 0 {
		t.Fatalf("lost retry prefix: %v adds=%d removes=%d", err, len(ra), len(rr))
	}

	// Verify the failed physical transaction leaves no reference edges.
	got, err := v.GetRefGraph().GetOutgoingRefs(t.Context(), "retry-owner")
	if err != nil || len(got) != 0 {
		t.Fatalf("failed graph leaked edges: %d %v", len(got), err)
	}

	// Retry the retained transition and verify every edge is stored.
	if err := v.GetRefGraph().ApplyRefBatch(t.Context(), ra, rr); err != nil {
		t.Fatal(err)
	}
	got, err = v.GetRefGraph().GetOutgoingRefs(t.Context(), "retry-owner")
	if err != nil || len(got) != len(adds) {
		t.Fatalf("retry edges=%d want=%d: %v", len(got), len(adds), err)
	}
}

func TestPublicationCloseJoinsDirectPreparation(t *testing.T) {
	// Start direct block preparation behind a physical transaction gate.
	v, raw := newPublicationTestVolume(t)
	gate := raw.arm(t)
	putDone := make(chan error, 1)
	go func() {
		_, _, err := v.PrepareOwnedBlock(t.Context(), "direct", []byte("pending construction"), leafOpts())
		putDone <- err
	}()

	// Wait for the direct preparation transaction to reach the gate.
	select {
	case <-gate.started:
	case <-time.After(time.Second):
		t.Fatal("direct transaction did not start")
	}

	// Verify Volume close waits for the blocked direct transaction.
	closed := make(chan error, 1)
	go func() { closed <- v.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("Close abandoned direct transaction: %v", err)
	case <-time.After(15 * time.Millisecond):
	}

	// Release preparation and verify both preparation and close finish.
	gate.unblock()
	for _, done := range []<-chan error{putDone, closed} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("direct close did not join")
		}
	}
}
