package block_gc

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestBlockCounts walks the counts through the lives of block trees: puts,
// owner edges, a replaced tree, a re-hold, a release past the cascade budget,
// sweeps that drop lists, and a missing record that puts the counts in doubt.
// After each step it checks every record against counts rebuilt from the
// child lists and owner edges.
func TestBlockCounts(t *testing.T) {
	// Open a graph whose counts are complete from birth.
	ctx := t.Context()
	rg := newTestRefGraph(t)
	state, err := rg.StartCounts(ctx)
	if err != nil || state.Phase != CountsComplete {
		t.Fatalf("start counts = %+v, %v", state, err)
	}
	node := func(name string) string { return BlockIRI(testBlockRef(t, name)) }

	// Store a leaf, then the parent naming it.
	leaf, parent := node("leaf"), node("parent")
	put(t, rg, leaf)
	put(t, rg, parent, leaf)
	wantCount(t, rg, leaf, BlockCount{Count: 1, Holds: true})
	wantCount(t, rg, parent, BlockCount{Queued: true, Holds: true})
	auditCounts(t, rg)

	// Root the parent under an owner.
	if err := rg.AddRef(ctx, "bucket/a", parent); err != nil {
		t.Fatal(err)
	}
	wantCount(t, rg, parent, BlockCount{Count: 1, Holds: true})
	auditCounts(t, rg)

	// Replace the tree: the new root shares the leaf and adds another. The old
	// root's release takes its hold on the leaf.
	next, other := node("next"), node("other")
	put(t, rg, other)
	put(t, rg, next, leaf, other)
	err = rg.ApplyRefBatch(ctx,
		[]RefEdge{{Subject: "bucket/a", Object: next}},
		[]RefEdge{{Subject: "bucket/a", Object: parent}},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantCount(t, rg, parent, BlockCount{Queued: true})
	wantCount(t, rg, leaf, BlockCount{Count: 1, Holds: true})
	auditCounts(t, rg)

	// Name the old root again within the grace period: it holds the leaf
	// again.
	if err := rg.AddRef(ctx, "bucket/b", parent); err != nil {
		t.Fatal(err)
	}
	wantCount(t, rg, parent, BlockCount{Count: 1, Holds: true})
	wantCount(t, rg, leaf, BlockCount{Count: 2, Holds: true})
	auditCounts(t, rg)

	// Build and root a fan wider than the cascade budget.
	fan := node("fan")
	children := make([]string, countCascadeBase+6)
	for i := range children {
		children[i] = node("fan/" + strconv.Itoa(i))
		put(t, rg, children[i], leaf)
	}
	put(t, rg, fan, children...)
	if err := rg.AddRef(ctx, "bucket/c", fan); err != nil {
		t.Fatal(err)
	}
	wantCount(t, rg, leaf, BlockCount{Count: 2 + uint64(len(children)), Holds: true})

	// Release it: the fan's root and the first children release, and the
	// rest queue still holding theirs.
	if err := rg.RemoveRef(ctx, "bucket/c", fan); err != nil {
		t.Fatal(err)
	}
	var holding int
	for _, child := range children {
		count, _, err := rg.GetBlockCount(ctx, child)
		if err != nil {
			t.Fatal(err)
		}
		if count.Count != 0 || !count.Queued {
			t.Fatalf("fan child count = %+v, want queued at zero", count)
		}
		if count.Holds {
			holding++
		}
	}
	if want := len(children) - (countCascadeBase - 1); holding != want {
		t.Fatalf("fan children still holding = %d, want %d", holding, want)
	}
	auditCounts(t, rg)

	// Sweep a held block: its count stays as a hole. Sweep a released one:
	// its record and queue entry go.
	if _, err := rg.RemoveNodeRefs(ctx, other, false); err != nil {
		t.Fatal(err)
	}
	wantCount(t, rg, other, BlockCount{Count: 1, Holds: true})
	if _, err := rg.RemoveNodeRefs(ctx, children[0], false); err != nil {
		t.Fatal(err)
	}
	if _, found, err := rg.GetBlockCount(ctx, children[0]); err != nil || found {
		t.Fatalf("swept child count found = %v, %v", found, err)
	}
	auditCounts(t, rg)

	// Lose a count record, then release a holder of it: the counts go into
	// doubt, the write commits, and nothing is counted after.
	deleteRecord(t, rg, graphKey(kindCount, leaf))

	// The audit names the block left without a count, for its list and its
	// holders, and stops at its bound.
	audit, err := rg.AuditCounts(ctx, 0, 1, 1<<20)
	if err != nil || audit.MismatchCount != 2 {
		t.Fatalf("audit after a lost count = %+v, %v", audit, err)
	}
	for _, mismatch := range audit.Mismatches {
		if mismatch.Block != leaf {
			t.Fatalf("audit mismatch %v, want the leaf", mismatch)
		}
	}
	if _, err := rg.AuditCounts(ctx, 0, 1, 1); err != ErrCountAuditFull {
		t.Fatalf("audit past its bound = %v, want ErrCountAuditFull", err)
	}

	// Release a holder of the block.
	if err := rg.RemoveRef(ctx, "bucket/b", parent); err != nil {
		t.Fatal(err)
	}
	state, err = rg.GetCountState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Phase != CountsDoubt || !strings.Contains(state.Doubt, "has no count") {
		t.Fatalf("count state = %+v, want doubt", state)
	}
	if owners, err := rg.GetIncomingRefs(ctx, parent); err != nil || slices.Contains(owners, "bucket/b") {
		t.Fatalf("parent owners = %v, %v, want bucket/b removed", owners, err)
	}
	if err := rg.AddRef(ctx, "bucket/d", next); err != nil {
		t.Fatal(err)
	}
	wantCount(t, rg, next, BlockCount{Count: 1, Holds: true})
}

// put stores block with children as a block put does: a staging edge to the
// block and an edge to each child.
func put(t *testing.T, rg *RefGraph, block string, children ...string) {
	t.Helper()
	adds := []RefEdge{{Subject: NodeUnreferenced, Object: block}}
	for _, child := range children {
		adds = append(adds, RefEdge{Subject: block, Object: child})
	}
	if err := rg.ApplyRefBatch(t.Context(), adds, nil); err != nil {
		t.Fatal(err)
	}
}

// wantCount checks a block's count record.
func wantCount(t *testing.T, rg *RefGraph, block string, want BlockCount) {
	t.Helper()
	got, found, err := rg.GetBlockCount(t.Context(), block)
	if err != nil {
		t.Fatal(err)
	}
	if !found || got != want {
		t.Fatalf("count of %s = %+v (found %v), want %+v", FormatIRI(block), got, found, want)
	}
}

// deleteRecord deletes one graph record.
func deleteRecord(t *testing.T, rg *RefGraph, key []byte) {
	t.Helper()
	if err := rg.deleteRecord(t.Context(), key); err != nil {
		t.Fatal(err)
	}
}

// auditCounts audits the counts whole and in three shares, and fails on any
// mismatch or on shares that do not add up to the whole.
func auditCounts(t *testing.T, rg *RefGraph) {
	// Audit the whole store.
	t.Helper()
	whole, err := rg.AuditCounts(t.Context(), 0, 1, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if whole.MismatchCount != 0 {
		t.Fatalf("count audit found %d mismatches: %v", whole.MismatchCount, whole.Mismatches)
	}

	// Audit it in shares, which must cover the same blocks.
	var blocks int
	for share := range uint32(3) {
		audit, err := rg.AuditCounts(t.Context(), share, 3, 1<<20)
		if err != nil || audit.MismatchCount != 0 {
			t.Fatalf("count audit of share %d = %+v, %v", share, audit, err)
		}
		blocks += audit.Blocks
	}
	if blocks != whole.Blocks {
		t.Fatalf("count audit shares compared %d blocks, want %d", blocks, whole.Blocks)
	}
}
