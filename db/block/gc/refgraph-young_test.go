package block_gc

import (
	"context"
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
