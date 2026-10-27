package block_gc

import (
	"context"
	"math/rand/v2"
	"strconv"
	"testing"

	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestMarkScanAcrossWrites checks that a scan mark split into transactions of
// a few records each, with random writes between them, keeps every block the
// roots reach once the sweep's rescue has run. A tail bound of a few blocks
// makes the scans both finish by point reads and overflow to another scan.
func TestMarkScanAcrossWrites(t *testing.T) {
	for seed := range uint64(300) {
		t.Run(strconv.FormatUint(seed, 10), func(t *testing.T) {
			testMarkScanAcrossWrites(t, rand.New(rand.NewPCG(seed, 2)))
		})
	}
}

// testMarkScanAcrossWrites runs one graph of TestMarkScanAcrossWrites.
func testMarkScanAcrossWrites(t *testing.T, rnd *rand.Rand) {
	// Open an empty graph.
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	rg := NewRefGraph(store)
	t.Cleanup(func() { rg.Close() })

	// Track the blocks the test creates.
	var blocks []string
	newBlock := func() string {
		block := BlockIRI(testBlockRef(t, "scan-"+strconv.Itoa(len(blocks))+"-"+strconv.FormatUint(rnd.Uint64(), 16)))
		blocks = append(blocks, block)
		return block
	}
	randomBlock := func() string { return blocks[rnd.IntN(len(blocks))] }

	// Create random blocks.
	var edges []RefEdge
	for range 4 + rnd.IntN(40) {
		edges = append(edges, RefEdge{Subject: NodeUnreferenced, Object: newBlock()})
	}

	// Link random pairs, give owners blocks, and root some owners.
	owners := []string{"bucket:a", "bucket:b", "object:c", "object:d"}
	for range rnd.IntN(2 * len(blocks)) {
		edges = append(edges, RefEdge{Subject: randomBlock(), Object: randomBlock()})
	}
	for range 1 + rnd.IntN(6) {
		edges = append(edges, RefEdge{Subject: owners[rnd.IntN(len(owners))], Object: randomBlock()})
	}
	for _, owner := range owners {
		if rnd.IntN(2) == 0 {
			edges = append(edges, RefEdge{Subject: NodeGCRoot, Object: owner})
		}
	}
	if err := rg.ApplyRefBatch(ctx, edges, nil); err != nil {
		t.Fatal(err)
	}

	// Between transactions, write at random: adopt old blocks from owners or
	// new blocks, link blocks, or drop owner and child edges.
	ownerEdges := func() []RefEdge {
		var out []RefEdge
		for _, owner := range owners {
			out2, err := rg.GetOutgoingRefs(ctx, owner)
			if err != nil {
				t.Fatal(err)
			}
			for _, object := range out2 {
				out = append(out, RefEdge{Subject: owner, Object: object})
			}
		}
		return out
	}
	between := func() error {
		var adds, removes []RefEdge
		switch rnd.IntN(5) {
		case 0:
			adds = append(adds, RefEdge{Subject: owners[rnd.IntN(len(owners))], Object: randomBlock()})
		case 1:
			parent := newBlock()
			adds = append(adds, RefEdge{Subject: NodeUnreferenced, Object: parent})
			for range rnd.IntN(3) {
				adds = append(adds, RefEdge{Subject: parent, Object: randomBlock()})
			}
			if rnd.IntN(2) == 0 {
				adds = append(adds, RefEdge{Subject: owners[rnd.IntN(len(owners))], Object: parent})
			}
		case 2:
			adds = append(adds, RefEdge{Subject: randomBlock(), Object: randomBlock()})
		case 3:
			if owned := ownerEdges(); len(owned) != 0 {
				removes = append(removes, owned[rnd.IntN(len(owned))])
			}
		case 4:
			parent := randomBlock()
			children, err := rg.GetOutgoingRefs(ctx, parent)
			if err != nil {
				return err
			}
			if len(children) != 0 {
				removes = append(removes, RefEdge{Subject: parent, Object: children[rnd.IntN(len(children))]})
			}
		}
		return rg.ApplyRefBatch(ctx, adds, removes)
	}

	// Open a pass, mark a record or a few at a time with a small tail, and
	// rescue.
	if err := rg.BeginBlockPass(ctx); err != nil {
		t.Fatal(err)
	}
	mark, err := rg.markScan(ctx, markLimit{records: 1 + rnd.IntN(3), tail: rnd.IntN(4)}, between)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := rg.RescueAdopted(ctx, mark, nil); err != nil {
		t.Fatal(err)
	}

	// Every block the roots reach now is live.
	want, err := rg.MarkBlocks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if mark.Unknown != 0 || want.Unknown != 0 {
		t.Fatalf("unknown %d, want %d; the test writes no untracked block", mark.Unknown, want.Unknown)
	}
	for _, block := range blocks {
		if want.Live.Has(block) && !mark.Live.Has(block) {
			t.Fatalf("block %q is reachable but the mark would sweep it", block)
		}
	}
}

// TestMarkScanUnknownAcrossOwnerWrite checks that a scan mark counts a reached
// block without a child list however late an owner adopts a block with a list.
// The settle step after the scans reads the owner edges again and must not
// count a block the edge adopted after the seeds, which the adopt log covers,
// against the unknown ones: an uncounted list would cancel the hole, and a
// sweep would take the mark as complete.
func TestMarkScanUnknownAcrossOwnerWrite(t *testing.T) {
	for at := range 12 {
		t.Run(strconv.Itoa(at), func(t *testing.T) {
			// Root an owner of a block whose list names a hole.
			ctx := context.Background()
			rg := NewRefGraph(store_kvtx_inmem.NewStore())
			t.Cleanup(func() { rg.Close() })
			parent := BlockIRI(testBlockRef(t, "scan-parent"))
			hole := BlockIRI(testBlockRef(t, "scan-hole"))
			late := BlockIRI(testBlockRef(t, "scan-late"))
			err := rg.ApplyRefBatch(ctx, []RefEdge{
				{Subject: NodeGCRoot, Object: "bucket:a"},
				{Subject: NodeUnreferenced, Object: parent},
				{Subject: "bucket:a", Object: parent},
				{Subject: parent, Object: hole},
			}, nil)
			if err != nil {
				t.Fatal(err)
			}

			// Adopt a block with a list from the owner between the
			// transactions, at a different point in each run.
			var calls int
			between := func() error {
				if calls++; calls != at+1 {
					return nil
				}
				return rg.ApplyRefBatch(ctx, []RefEdge{
					{Subject: NodeUnreferenced, Object: late},
					{Subject: "bucket:a", Object: late},
				}, nil)
			}
			mark, err := rg.markScan(ctx, markLimit{records: 1}, between)
			if err != nil {
				t.Fatal(err)
			}

			// The hole is the one unknown block, and the mark lists it.
			if mark.Unknown != 1 || !mark.Awaitable() {
				t.Fatalf("unknown %d with %d listed, want the hole alone", mark.Unknown, len(mark.awaiting))
			}
		})
	}
}
