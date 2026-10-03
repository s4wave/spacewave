package block_gc

import (
	"context"
	"strconv"
	"testing"

	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

func BenchmarkRefGraphRemoveBatchLargeStore(b *testing.B) {
	// Define the large graph and removal sample sizes.
	const (
		seedEdgeCount = 100_000
		removeCount   = 600
	)

	// Prepare the large graph seed edges.
	ctx := context.Background()
	rg := newLargeBenchRefGraph(b, ctx)
	seed := make([]RefEdge, seedEdgeCount)
	for i := range seed {
		seed[i] = RefEdge{
			Subject: "bench/large/subject/" + strconv.Itoa(i),
			Object:  "bench/large/object/" + strconv.Itoa(i),
		}
	}

	// Seed through the owner API so the timed removals see the same durable
	// graph a live store would present them.
	if err := rg.ApplyRefBatch(ctx, seed, nil); err != nil {
		b.Fatal(err)
	}

	// Verify graph seeding retains the first expected edge.
	if found, err := rg.hasRef(ctx, seed[0].Subject, seed[0].Object); err != nil {
		b.Fatal(err)
	} else if !found {
		b.Fatalf("seeding %d edges left %v out of the graph", seedEdgeCount, seed[0])
	}

	// Measure repeated removals from the seeded large reference graph.
	removes := seed[:removeCount]
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		// Measure the removal batch against the large graph.
		if err := rg.ApplyRefBatch(ctx, nil, removes); err != nil {
			b.Fatal(err)
		}

		// Restore removed edges outside the timed sample.
		b.StopTimer()
		if err := rg.ApplyRefBatch(ctx, removes, nil); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}

	// Report graph size and removal workload per operation.
	b.ReportMetric(float64(seedEdgeCount), "seed_edges/op")
	b.ReportMetric(float64(removeCount), "remove_edges/op")
}

func newLargeBenchRefGraph(b *testing.B, ctx context.Context) *RefGraph {
	// Open the large benchmark reference graph and register its cleanup.
	b.Helper()
	rg, err := NewRefGraph(ctx, store_kvtx_inmem.NewStore(), []byte("gc/"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { rg.Close() })
	return rg
}
