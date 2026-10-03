package block_gc

import (
	"slices"
	"strconv"
	"testing"

	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestRefBatchLookup shares snapshots across an entire mixed removal batch
// while retaining exact membership, input order, and add-before-remove rules.
func TestRefBatchLookup(t *testing.T) {
	// Open the reference graph with read-snapshot accounting.
	ctx := t.Context()
	store := &countingStore{Store: store_kvtx_inmem.NewStore()}
	rg, err := NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	defer rg.Close()

	// Prepare mixed present and absent ownership edges.
	var adds, removes, want []RefEdge
	for i := range 512 {
		edge := RefEdge{Subject: "owner-" + strconv.Itoa(i), Object: "target"}
		removes = append(removes, edge)
		if i%2 == 0 {
			adds = append(adds, edge)
			want = append(want, edge)
		}
	}

	// Seed the present ownership edges.
	if err := rg.ApplyRefBatch(ctx, adds, nil); err != nil {
		t.Fatal(err)
	}

	// Include a newly added edge and duplicate in the removal expectations.
	added := RefEdge{Subject: "new owner", Object: "new target"}
	removes = append(removes, added, adds[0])
	want = append(want, added, adds[0])

	// Filter mixed removals while counting read snapshots.
	before := store.opens.Load()
	_, got, err := rg.filterRefChanges(ctx, []RefEdge{added}, removes)
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("mixed removals: count=%d err=%v", len(got), err)
	}

	// Verify filtering shares snapshots across the removal batch.
	if opened := store.opens.Load() - before; opened > 3 {
		t.Fatalf("batch opened %d storage transactions", opened)
	}
}
