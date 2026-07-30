package block_gc

import (
	"context"
	"strconv"
	"testing"

	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

func BenchmarkRefGraphRemoveBatchLargeStore(b *testing.B) {
	const (
		seedEdgeCount = 100_000
		removeCount   = 600
	)

	ctx := context.Background()
	rg := newLargeBenchRefGraph(b, ctx)
	seed := make([]RefEdge, seedEdgeCount)
	for i := range seed {
		seed[i] = RefEdge{
			Subject: "bench/large/subject/" + strconv.Itoa(i),
			Object:  "bench/large/object/" + strconv.Itoa(i),
		}
	}
	// Seed through the owner API. Writing to the handle directly leaves the
	// edge index empty, so the timed removals would run against an index
	// holding only what the benchmark itself restores between iterations.
	if err := rg.ApplyRefBatch(ctx, seed, nil); err != nil {
		b.Fatal(err)
	}
	if got := len(rg.edgeIndex); got != seedEdgeCount {
		b.Fatalf("seeded %d edges but the index holds %d", seedEdgeCount, got)
	}
	removes := seed[:removeCount]

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := rg.ApplyRefBatch(ctx, nil, removes); err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
		if err := rg.ApplyRefBatch(ctx, removes, nil); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}
	b.ReportMetric(float64(seedEdgeCount), "seed_edges/op")
	b.ReportMetric(float64(removeCount), "remove_edges/op")
}

func newLargeBenchRefGraph(b *testing.B, ctx context.Context) *RefGraph {
	b.Helper()

	rg, err := NewRefGraph(ctx, store_kvtx_inmem.NewStore(), []byte("gc/"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { rg.Close() })
	return rg
}
