package block_gc

import (
	"context"
	"slices"
	"testing"

	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestTraceOwners checks that a trace finds the owned path to a block past an
// unreached parent, and reports no owner for a block nothing owned reaches.
func TestTraceOwners(t *testing.T) {
	// A bucket owns top, which reaches leaf through mid. An unowned stray
	// also names leaf, and orphan has no parent.
	ctx := context.Background()
	rg := NewRefGraph(store_kvtx_inmem.NewStore())
	t.Cleanup(func() { rg.Close() })
	block := func(name string) string { return BlockIRI(testBlockRef(t, name)) }
	top, mid, leaf, stray, orphan := block("top"), block("mid"), block("leaf"), block("stray"), block("orphan")
	bucket := BucketIRI("b")
	edges := []RefEdge{
		{Subject: bucket, Object: top},
		{Subject: top, Object: mid},
		{Subject: mid, Object: leaf},
		{Subject: NodeUnreferenced, Object: stray},
		{Subject: stray, Object: leaf},
		{Subject: NodeUnreferenced, Object: orphan},
	}
	if err := rg.ApplyRefBatch(ctx, edges, nil); err != nil {
		t.Fatal(err)
	}

	traces, err := rg.TraceOwners(ctx, []string{leaf, orphan}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(traces) != 2 {
		t.Fatalf("got %d traces", len(traces))
	}
	if got := traces[0]; got.Owner != bucket || !slices.Equal(got.Path, []string{top, mid, leaf}) {
		t.Fatalf("leaf trace %+v", got)
	}
	if got := traces[1]; got.Owner == bucket || len(got.Path) > 1 {
		t.Fatalf("orphan trace %+v", got)
	}
}
