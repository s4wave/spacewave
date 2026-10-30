package block_gc

import (
	"encoding/binary"
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
	complete, err := rg.ClaimCountsComplete(ctx)
	if err != nil || !complete {
		t.Fatalf("claim counts complete = %v, %v", complete, err)
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
	if err := rg.RemoveRef(ctx, "bucket/b", parent); err != nil {
		t.Fatal(err)
	}
	state, err := rg.GetCountState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Complete || !strings.Contains(state.Doubt, "has no count") {
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

// auditCounts rebuilds the counts from the child lists and owner edges and
// checks them against the stored count records and free queue.
func auditCounts(t *testing.T, rg *RefGraph) {
	// Read every record.
	t.Helper()
	records := snapshotRefGraphRecords(t, rg.store)

	// Decode the count records and free queue entries.
	counts := make(map[string]countRecord)
	queued := make(map[uint64]string)
	for key, value := range records {
		switch key[0] {
		case kindCount:
			block, _, err := readNode([]byte(key[1:]))
			if err != nil {
				t.Fatal(err)
			}
			rec, err := decodeCountRecord(value)
			if err != nil {
				t.Fatal(err)
			}
			counts[block] = rec
		case kindFree:
			block, _, err := readNode(value[8:])
			if err != nil {
				t.Fatal(err)
			}
			queued[binary.BigEndian.Uint64([]byte(key[1:]))] = block
		}
	}

	// Rebuild each block's count from the owner edges of nodes that are not
	// blocks and the lists of the blocks that hold.
	want := make(map[string]uint64)
	for key := range records {
		if key[0] != kindIncoming {
			continue
		}
		object, rest, err := readNode([]byte(key[1:]))
		if err != nil {
			t.Fatal(err)
		}
		subject, _, err := readNode(rest)
		if err != nil {
			t.Fatal(err)
		}
		if isBlockNode(object) && subject != NodeUnreferenced {
			want[object]++
		}
	}
	for key, value := range records {
		if key[0] != kindChildren {
			continue
		}
		block, _, err := readNode([]byte(key[1:]))
		if err != nil {
			t.Fatal(err)
		}
		rec, ok := counts[block]
		if !ok {
			t.Fatalf("child list of %s has no count", FormatIRI(block))
		}
		if rec.count == 0 && !rec.holds {
			continue
		}
		children, err := decodeChildren(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, child := range children {
			want[child]++
		}
	}

	// Compare, and check each zero count has its one queue entry.
	for block, rec := range counts {
		if rec.count != want[block] {
			t.Fatalf("count of %s = %d, want %d", FormatIRI(block), rec.count, want[block])
		}
		if rec.count == 0 && queued[rec.seq] != block {
			t.Fatalf("zero count of %s has no queue entry %d", FormatIRI(block), rec.seq)
		}
		delete(want, block)
	}
	for block, n := range want {
		if n != 0 {
			t.Fatalf("%s has %d holders and no count", FormatIRI(block), n)
		}
	}
	var zero int
	for _, rec := range counts {
		if rec.count == 0 {
			zero++
		}
	}
	if zero != len(queued) {
		t.Fatalf("free queue has %d entries for %d zero counts", len(queued), zero)
	}
}
