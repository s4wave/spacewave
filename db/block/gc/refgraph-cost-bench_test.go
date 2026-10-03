package block_gc

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/cayley/graph"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// BenchmarkRefGraphRemovalCostByStoreSize reports the owner-held preparation
// and mutation costs for the common removal workload. Odd-indexed removals are
// missing edges; even-indexed removals are seeded edges.
func BenchmarkRefGraphRemovalCostByStoreSize(b *testing.B) {
	// Prepare the context for removal cost comparisons.
	ctx := context.Background()
	for _, backend := range []struct {
		name    string
		generic bool
	}{
		{name: "native-cayley"},
		{name: "generic-fallback", generic: true},
	} {
		for _, edgeCount := range worldCostCheckpoints() {
			b.Run(backend.name+"/edges="+strconv.Itoa(edgeCount), func(b *testing.B) {
				// Measure graph preparation and mutation across fresh fixtures.
				var preparation, mutation, ownerHeld time.Duration
				for iteration := range b.N {
					// Prepare the reference graph outside the timed sample.
					b.StopTimer()
					rg, removes := newCostRefGraph(b, ctx, edgeCount, iteration, backend.generic)
					b.StartTimer()

					// Prepare reference changes while holding the graph write lock.
					rg.writeMu.Lock()
					start := time.Now()
					adds, removes, err := rg.prepareRefBatch(ctx, nil, removes, true)
					prepDuration := time.Since(start)
					if err != nil {
						rg.writeMu.Unlock()
						b.Fatal(err)
					}

					// Apply the prepared graph changes and release the write lock.
					start = time.Now()
					_, _, err = rg.applyRefBatchSliceLocked(ctx, adds, removes)
					mutationDuration := time.Since(start)
					rg.writeMu.Unlock()
					if err != nil {
						b.Fatal(err)
					}

					// Accumulate preparation, mutation, and lock-hold durations.
					preparation += prepDuration
					mutation += mutationDuration
					ownerHeld += prepDuration + mutationDuration

					// Close the reference graph outside the timed sample.
					b.StopTimer()
					if err := rg.Close(); err != nil {
						b.Fatal(err)
					}
				}

				// Report mean preparation, mutation, and lock-hold costs.
				if b.N != 0 {
					denom := float64(b.N)
					b.ReportMetric(float64(preparation.Nanoseconds())/denom, "prepare_ns/op")
					b.ReportMetric(float64(mutation.Nanoseconds())/denom, "mutation_ns/op")
					b.ReportMetric(float64(ownerHeld.Nanoseconds())/denom, "owner_held_ns/op")
				}

				// Report the removal workload size.
				b.ReportMetric(float64(edgeCount), "removal_edges/op")
			})
		}
	}
}

func worldCostCheckpoints() []int {
	return []int{1, 8, 16, 32, 64, 96, 128}
}

func newCostRefGraph(
	b *testing.B,
	ctx context.Context,
	edgeCount, iteration int,
	generic bool,
) (*RefGraph, []RefEdge) {
	// Open the reference graph for the selected backend.
	b.Helper()
	rg, err := NewRefGraph(ctx, store_kvtx_inmem.NewStore(), []byte("gc/"))
	if err != nil {
		b.Fatal(err)
	}
	if generic {
		rg.handle.QuadStore = genericRefGraphQuadStore{QuadStore: rg.handle.QuadStore}
	}

	// Prepare removal edges and seed every other edge into the graph.
	removes := make([]RefEdge, edgeCount)
	prefix := "cost/" + strconv.Itoa(iteration) + "/"
	for i := range removes {
		// Create the removal edge for this fixture position.
		removes[i] = RefEdge{
			Subject: prefix + "subject/" + strconv.Itoa(i),
			Object:  prefix + "object/" + strconv.Itoa(i),
		}

		// Seed the even-position edge into the graph.
		if i%2 == 0 {
			if err := rg.AddRef(ctx, removes[i].Subject, removes[i].Object); err != nil {
				rg.Close()
				b.Fatal(err)
			}
		}
	}
	return rg, removes
}

type genericRefGraphQuadStore struct {
	graph.QuadStore
}

var _ graph.QuadStore = (genericRefGraphQuadStore{})
