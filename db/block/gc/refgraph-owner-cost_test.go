package block_gc

import (
	"context"
	"strconv"
	"testing"
	"time"

	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

func TestRemoveSharedOwnerReadCost(t *testing.T) {
	small := removeSharedOwnerReadCost(t, 16)
	large := removeSharedOwnerReadCost(t, 4096)
	t.Logf("owner removal reads: 16 owners=%d; 4096 owners=%d", small, large)
	if large > small+32 {
		t.Fatalf("removing one owner scans other owners: %d reads at 16 owners, %d at 4096", small, large)
	}
}

func removeSharedOwnerReadCost(t *testing.T, owners int) int64 {
	t.Helper()
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	seed, err := NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	edges := make([]RefEdge, owners)
	for i := range edges {
		edges[i] = RefEdge{Subject: "owner/" + strconv.Itoa(i), Object: "shared"}
	}
	if err := seed.ApplyRefBatch(ctx, edges, nil); err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen without warmed names so the cost includes the durable lookup.
	counted := &countingStore{Store: store}
	rg, err := NewRefGraph(ctx, counted, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	defer rg.Close()
	before := counted.reads.Load()
	if err := rg.ApplyRefBatch(ctx, nil, edges[:1]); err != nil {
		t.Fatal(err)
	}
	reads := counted.reads.Load() - before
	if found, err := rg.hasRef(ctx, edges[0].Subject, "shared"); err != nil || found {
		t.Fatalf("removed owner remains: found=%v err=%v", found, err)
	}
	if found, err := rg.HasIncomingRefs(ctx, "shared"); err != nil || !found {
		t.Fatalf("remaining owner lost: found=%v err=%v", found, err)
	}
	if found, err := rg.hasRef(ctx, NodeUnreferenced, "shared"); err != nil || found {
		t.Fatalf("shared object marked orphaned: found=%v err=%v", found, err)
	}
	return reads
}

// TestRefBatchReplayDoesNotCommit verifies idempotent ownership replay without
// another durable write, including removal of already absent staging edges.
func TestRefBatchReplayDoesNotCommit(t *testing.T) {
	ctx := t.Context()
	store := &refGraphTrackingStore{Store: store_kvtx_inmem.NewStore()}
	rg, err := NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	defer rg.Close()
	adds := make([]RefEdge, 722)
	removes := make([]RefEdge, len(adds))
	for i := range adds {
		object := "block/" + strconv.Itoa(i)
		adds[i] = RefEdge{Subject: "parent", Object: object}
		removes[i] = RefEdge{Subject: NodeUnreferenced, Object: object}
	}
	if err := rg.ApplyRefBatch(ctx, adds, removes); err != nil {
		t.Fatal(err)
	}
	before := store.commits.Load()
	start := time.Now()
	for range 5 {
		if err := rg.ApplyRefBatch(ctx, adds, removes); err != nil {
			t.Fatal(err)
		}
	}
	commits := store.commits.Load() - before
	t.Logf("five replays of 722 references: %s; commits=%d", time.Since(start), commits)
	if commits != 0 {
		t.Fatalf("unchanged ownership committed %d transactions", commits)
	}
	found, err := rg.hasRefs(ctx, adds)
	if err != nil {
		t.Fatal(err)
	}
	for i, present := range found {
		if !present {
			t.Fatalf("replay lost reference %d", i)
		}
	}
}
