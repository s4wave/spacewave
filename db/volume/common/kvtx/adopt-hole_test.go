package kvtx

import (
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestAdoptKeepsTheChildrenOfAHole adopts a Volume whose root names R, a block
// the Volume neither stores nor lists, as a World seeded by a copy that left
// out the reference graph and then evicted R to a pack does. R's child C is
// stored. The adoption cannot know R's children, so it must not count C as
// unheld: it refuses, or it keeps C, and the next pass frees nothing R names.
func TestAdoptKeepsTheChildrenOfAHole(t *testing.T) {
	// Store R -> C under main in a Volume that predates adoption.
	store := store_kvtx_inmem.NewStore()
	setUnadopted(t, store)
	v, clk := openUnadoptedVolume(t, store)
	ctx := t.Context()
	tr := testTree{}
	tr.add(t, "C")
	r := tr.add(t, "R", "C")
	if err := v.SetBucketRoots(ctx, "b", tr.entries("C", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
		t.Fatal(err)
	}

	// Lose R's bytes and child list, keeping C.
	punchHole(t, v, r)
	requireStored(t, v, tr, false, "R")
	requireStored(t, v, tr, true, "C")

	// Adopt, then collect past the grace period. C stays either way.
	res, err := v.Adopt(ctx)
	clk.Add(block_gc.DefaultGrace + time.Second)
	collectAll(t, v)
	if _, found, gerr := v.GetBlock(ctx, tr["C"].Ref); gerr != nil || !found {
		t.Fatalf("adopt = %+v, %v; then C stored=%v (%v), want C kept while main names R", res, err, found, gerr)
	}
}
