package block_gc

import (
	"context"
	"errors"
	"slices"
	"testing"

	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestPromoteYoungCutShortNotReady checks that a promotion cut short leaves
// the young log not complete. Its first commit makes a parent old while the
// child it names, whose list a record after the bound created, is still
// young and no record lists the edge, so a minor pass then would sweep the
// live child.
func TestPromoteYoungCutShortNotReady(t *testing.T) {
	// A graph born empty is minor-ready.
	ctx := context.Background()
	rg := NewRefGraph(store_kvtx_inmem.NewStore())
	t.Cleanup(func() { rg.Close() })
	if ready, err := rg.ClaimYoungReady(ctx); err != nil || !ready {
		t.Fatalf("claim on an empty graph: ready=%v err=%v", ready, err)
	}

	// Write the parent before the bound and the child's list after it.
	owner := BucketIRI("owner")
	parent, other := BlockIRI(testBlockRef(t, "young-parent")), BlockIRI(testBlockRef(t, "young-other"))
	child, leaf := BlockIRI(testBlockRef(t, "young-child")), BlockIRI(testBlockRef(t, "young-leaf"))
	apply := func(edges ...RefEdge) {
		t.Helper()
		if err := rg.ApplyRefBatch(ctx, edges, nil); err != nil {
			t.Fatal(err)
		}
	}
	apply(RefEdge{Subject: owner, Object: parent}, RefEdge{Subject: parent, Object: child})
	apply(RefEdge{Subject: owner, Object: other})
	bound, err := rg.YoungPosition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := rg.BeginBlockPass(ctx); err != nil {
		t.Fatal(err)
	}
	apply(RefEdge{Subject: child, Object: leaf})
	mark, err := rg.MarkBlocks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range []string{parent, other, child} {
		if !mark.Live.Has(b) {
			t.Fatalf("block %q not reached", b)
		}
	}

	// A promotion stopped after one record is not minor-ready.
	stop := func() bool { return true }
	done, err := rg.PromoteYoung(ctx, mark, bound, stop)
	if err != nil || done {
		t.Fatalf("stopped promotion: done=%v err=%v", done, err)
	}
	if ready, err := rg.YoungReady(ctx); err != nil || ready {
		t.Fatalf("after a promotion cut short: ready=%v err=%v", ready, err)
	}
	if ready, err := rg.ClaimYoungReady(ctx); err != nil || ready {
		t.Fatalf("claim on a tracked graph: ready=%v err=%v", ready, err)
	}

	// Finishing the promotion makes it minor-ready, and a minor mark keeps
	// the child and its leaf.
	for !done {
		if done, err = rg.PromoteYoung(ctx, mark, bound, stop); err != nil {
			t.Fatal(err)
		}
	}
	if ready, err := rg.YoungReady(ctx); err != nil || !ready {
		t.Fatalf("after a full promotion: ready=%v err=%v", ready, err)
	}
	young, err := rg.MarkYoung(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(young.Unreached, child) || slices.Contains(young.Unreached, leaf) {
		t.Fatalf("minor mark leaves live blocks unreached: %v", young.Unreached)
	}
	if err := rg.EndBlockPass(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestPromoteKeepsSettledHoleYoung checks that a block a pass settled as a
// hole, and that a write stored after the pass's last rescue and before the
// promotion, stays young, and that a minor mark reaches it and the block it names from
// the namer the settling logged. The pass reached the hole but never walked a
// list of its, so promoting it would leave an old block naming a young block
// through no record.
func TestPromoteKeepsSettledHoleYoung(t *testing.T) {
	// A graph born empty holds an owned parent whose list names a hole.
	ctx := context.Background()
	rg := NewRefGraph(store_kvtx_inmem.NewStore())
	t.Cleanup(func() { rg.Close() })
	if ready, err := rg.ClaimYoungReady(ctx); err != nil || !ready {
		t.Fatalf("claim on an empty graph: ready=%v err=%v", ready, err)
	}
	owner := BucketIRI("owner")
	parent, hole := BlockIRI(testBlockRef(t, "hole-parent")), BlockIRI(testBlockRef(t, "hole"))
	child := BlockIRI(testBlockRef(t, "hole-child"))
	apply := func(edges ...RefEdge) {
		t.Helper()
		if err := rg.ApplyRefBatch(ctx, edges, nil); err != nil {
			t.Fatal(err)
		}
	}
	apply(RefEdge{Subject: owner, Object: parent}, RefEdge{Subject: parent, Object: hole})

	// A pass marks the hole unknown and settles it.
	bound, err := rg.YoungPosition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := rg.BeginBlockPass(ctx); err != nil {
		t.Fatal(err)
	}
	mark, err := rg.MarkBlocks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if mark.Unknown != 1 {
		t.Fatalf("unknown=%d", mark.Unknown)
	}
	absent := func(string) (bool, error) { return false, nil }
	if settled, err := mark.SettleAbsent(absent); err != nil || settled != 1 {
		t.Fatalf("settle: settled=%d err=%v", settled, err)
	}
	if err := rg.LogSettled(ctx, mark); err != nil {
		t.Fatal(err)
	}

	// A write with no owner stores the hole and its child after the last
	// rescue and before the promotion.
	apply(RefEdge{Subject: NodeUnreferenced, Object: child})
	apply(RefEdge{Subject: NodeUnreferenced, Object: hole}, RefEdge{Subject: hole, Object: child})
	if err := rg.EndBlockPass(ctx); err != nil {
		t.Fatal(err)
	}
	for done := false; !done; {
		if done, err = rg.PromoteYoung(ctx, mark, bound, nil); err != nil {
			t.Fatal(err)
		}
	}

	// A minor mark reaches the hole and its child.
	if err := rg.BeginBlockPass(ctx); err != nil {
		t.Fatal(err)
	}
	young, err := rg.MarkYoung(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range []string{hole, child} {
		if !young.Live.Has(b) || slices.Contains(young.Unreached, b) {
			t.Fatalf("minor mark leaves %q unreached: unreached=%v", b, young.Unreached)
		}
	}
	if err := rg.EndBlockPass(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestMarkYoungStopsAtFullLog checks that a minor mark stops before reading
// owners once the young log lists more than maxYoungBlocks blocks, and marks
// a log at the bound.
func TestMarkYoungStopsAtFullLog(t *testing.T) {
	// Open a graph born empty, so its young log is complete.
	ctx := context.Background()
	rg := NewRefGraph(store_kvtx_inmem.NewStore())
	t.Cleanup(func() { rg.Close() })
	if ready, err := rg.ClaimYoungReady(ctx); err != nil || !ready {
		t.Fatalf("claim on an empty graph: ready=%v err=%v", ready, err)
	}

	// Log three young blocks under one owner.
	owner := BucketIRI("owner")
	parent := BlockIRI(testBlockRef(t, "full-parent"))
	child := BlockIRI(testBlockRef(t, "full-child"))
	leaf := BlockIRI(testBlockRef(t, "full-leaf"))
	edges := []RefEdge{{Subject: owner, Object: parent}, {Subject: parent, Object: child}, {Subject: child, Object: leaf}}
	if err := rg.ApplyRefBatch(ctx, edges, nil); err != nil {
		t.Fatal(err)
	}

	// Count the blocks the log lists.
	young, err := rg.MarkYoung(ctx)
	if err != nil {
		t.Fatal(err)
	}
	listed := young.YoungBlocks
	if listed == 0 {
		t.Fatal("no young blocks logged")
	}

	// A bound below the log stops the mark; a bound at it marks.
	defer func(n int) { maxYoungBlocks = n }(maxYoungBlocks)
	maxYoungBlocks = listed - 1
	if _, err := rg.MarkYoung(ctx); !errors.Is(err, ErrYoungLogFull) {
		t.Fatalf("mark past the bound: err=%v", err)
	}
	maxYoungBlocks = listed
	young, err = rg.MarkYoung(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range []string{parent, child, leaf} {
		if !young.Live.Has(b) {
			t.Fatalf("block %q not reached", b)
		}
	}
}
