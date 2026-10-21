package block_gc

import (
	"context"
	"testing"

	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestRescueAdoptedUntrackedParent checks that a rescue counts a block adopted
// during a pass that has no child list as unknown: a parent stored untracked
// after the mark may name a child the mark found unreached.
func TestRescueAdoptedUntrackedParent(t *testing.T) {
	// A tracked child that nothing owns.
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	rg := NewRefGraph(store)
	t.Cleanup(func() { rg.Close() })
	parent, child := BlockIRI(testBlockRef(t, "untracked-parent")), BlockIRI(testBlockRef(t, "untracked-child"))
	if err := rg.ApplyRefBatch(ctx, []RefEdge{{Subject: NodeUnreferenced, Object: child}}, nil); err != nil {
		t.Fatal(err)
	}

	// Open a pass and mark: the child is unreached.
	if err := rg.BeginBlockPass(ctx); err != nil {
		t.Fatal(err)
	}
	mark, err := rg.MarkBlocks(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// A bucket adopts the parent, stored without a child list, which
	// references the child.
	if err := rg.AddRef(ctx, "bucket", parent); err != nil {
		t.Fatal(err)
	}
	deleteChildList(t, store, parent)
	if _, err := rg.RescueAdopted(ctx, mark); err != nil {
		t.Fatal(err)
	}

	// The rescue reports the parent, so the sweep removes nothing.
	if mark.Unknown != 1 || len(mark.UnknownSample) != 1 || mark.UnknownSample[0] != parent {
		t.Fatalf("unknown %d %v, want the parent", mark.Unknown, mark.UnknownSample)
	}
}
