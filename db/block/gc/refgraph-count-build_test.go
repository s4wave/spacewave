package block_gc

import (
	"context"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// countBuildSeeds is the number of write sequences TestCountBuild runs.
const countBuildSeeds = 8

// TestCountBuild builds the counts of graphs written before they were kept,
// one record per slice, with writes between the slices that add and replace
// trees, add and drop owners, grow lists and sweep blocks. It audits the
// counts the build has reached after every write and slice. A block stored without a child list puts the build in
// doubt; one the store never held is a hole the build keeps.
func TestCountBuild(t *testing.T) {
	for seed := range uint64(countBuildSeeds) {
		t.Run(strconv.FormatUint(seed, 10), func(t *testing.T) {
			// Open a graph and a random source.
			ctx := t.Context()
			rg := newTestRefGraph(t)
			rnd := rand.New(rand.NewPCG(seed, 1))

			// Name new blocks, pick existing ones, and store a new block
			// naming a few.
			var blocks []string
			node := func() string {
				b := BlockIRI(testBlockRef(t, strconv.FormatUint(seed, 10)+"/"+strconv.Itoa(len(blocks))))
				blocks = append(blocks, b)
				return b
			}
			pick := func() string { return blocks[rnd.IntN(len(blocks))] }
			grow := func() {
				var children []string
				for range rnd.IntN(4) {
					children = append(children, pick())
				}
				put(t, rg, node(), children...)
			}

			// Write a forest and owners, plus an owner of a block without a
			// list, before the counts start.
			put(t, rg, node())
			for range 24 {
				grow()
			}
			for i := range 8 {
				if err := rg.AddRef(ctx, "bucket/"+strconv.Itoa(i), pick()); err != nil {
					t.Fatal(err)
				}
			}
			bare := BlockIRI(testBlockRef(t, "bare"))
			if err := rg.AddRef(ctx, "bucket/bare", bare); err != nil {
				t.Fatal(err)
			}

			// Start the build and run it a record at a time, writing between
			// the slices.
			state, err := rg.StartCounts(ctx)
			if err != nil || state.Phase != CountsWalking {
				t.Fatalf("start counts = %+v, %v", state, err)
			}
			notStored := func(context.Context, *block.BlockRef) (bool, error) { return false, nil }
			for state.Building() {
				switch rnd.IntN(6) {
				case 0:
					grow()
				case 1:
					err = rg.AddRef(ctx, "bucket/"+strconv.Itoa(rnd.IntN(8)), pick())
				case 2:
					err = rg.RemoveRef(ctx, "bucket/"+strconv.Itoa(rnd.IntN(8)), pick())
				case 3:
					err = rg.ApplyRefBatch(ctx,
						[]RefEdge{{Subject: "bucket/0", Object: pick()}},
						[]RefEdge{{Subject: "bucket/0", Object: pick()}},
					)
				case 4:
					put(t, rg, pick(), pick())
				case 5:
					_, err = rg.RemoveNodeRefs(ctx, pick(), false)
				}
				if err != nil {
					t.Fatal(err)
				}
				auditCounts(t, rg)
				state, err = rg.BuildCounts(ctx, 1, func() bool { return false }, notStored)
				if err != nil {
					t.Fatal(err)
				}
				auditCounts(t, rg)
			}
			if state.Phase != CountsComplete {
				t.Fatalf("count state = %+v, want complete", state)
			}
			auditCounts(t, rg)
			wantCount(t, rg, bare, BlockCount{Count: 1, Holds: true})

			// Rebuild with the bare block stored: the counts are in doubt.
			deleteRecord(t, rg, countsKey)
			for _, key := range countRecordKeys(t, rg) {
				deleteRecord(t, rg, key)
			}
			state, err = rg.StartCounts(ctx)
			stored := func(_ context.Context, ref *block.BlockRef) (bool, error) {
				return BlockIRI(ref) == bare, nil
			}
			for err == nil && state.Building() {
				state, err = rg.BuildCounts(ctx, 64, func() bool { return false }, stored)
			}
			if err != nil || state.Phase != CountsDoubt || !strings.Contains(state.Doubt, "stored without a child list") {
				t.Fatalf("count state = %+v, %v, want doubt", state, err)
			}
		})
	}
}

// countRecordKeys lists the keys of the count records, free queue entries and
// free queue head.
func countRecordKeys(t *testing.T, rg *RefGraph) [][]byte {
	t.Helper()
	var keys [][]byte
	for key := range snapshotRefGraphRecords(t, rg.store) {
		switch key[0] {
		case kindCount, kindFree, kindFreeHead:
			keys = append(keys, []byte(key))
		}
	}
	return keys
}
