package block_gc

import (
	"context"
	"strconv"
	"testing"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/quad"
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
	tx := graph.NewTransactionN(seedEdgeCount)
	for _, edge := range seed {
		tx.AddQuad(quad.Make(quad.IRI(edge.Subject), quad.IRI(PredGCRef), quad.IRI(edge.Object), nil))
	}
	if err := rg.handle.ApplyTransaction(ctx, tx); err != nil {
		b.Fatal(err)
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
