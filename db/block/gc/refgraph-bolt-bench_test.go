//go:build !js && !wasip1

package block_gc

import (
	"path/filepath"
	"strconv"
	"testing"

	store_kvtx_bolt "github.com/s4wave/spacewave/db/store/kvtx/bolt"
)

// BenchmarkRefGraphDurableBatch includes the real Bolt commit and fsync path
// used by a native block-store drain. Database setup is outside the sample.
func BenchmarkRefGraphDurableBatch(b *testing.B) {
	// Open the Bolt store for durable graph measurements.
	ctx := b.Context()
	store, err := store_kvtx_bolt.Open(filepath.Join(b.TempDir(), "refs.db"), 0o600, nil, []byte("test"))
	if err != nil {
		b.Fatal(err)
	}
	defer store.GetDB().Close()

	// Open the durable reference graph and register its cleanup.
	rg, err := NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		b.Fatal(err)
	}
	defer rg.Close()

	// Measure durable additions and allocations with prepared edge storage.
	edges := make([]RefEdge, 4096)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := range b.N {
		// Prepare fresh graph edges outside the timed sample.
		b.StopTimer()
		for i := range edges {
			edges[i] = RefEdge{Subject: "batch-" + strconv.Itoa(iteration), Object: "block-" + strconv.Itoa(i)}
		}

		// Measure the durable graph batch commit.
		b.StartTimer()
		if err := rg.ApplyRefBatch(ctx, edges, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRefGraphOwnershipTransfer compares the same prepared transition
// under two synced commits and the combined commit, using a fresh Bolt store.
func BenchmarkRefGraphOwnershipTransfer(b *testing.B) {
	for _, separate := range []bool{true, false} {
		// Choose the commit shape for ownership transfer measurements.
		name := "combined"
		if separate {
			name = "separate"
		}
		b.Run(name, func(b *testing.B) {
			// Open the Bolt store for durable graph measurements.
			ctx := b.Context()
			store, err := store_kvtx_bolt.Open(filepath.Join(b.TempDir(), "refs.db"), 0o600, nil, []byte("test"))
			if err != nil {
				b.Fatal(err)
			}
			defer store.GetDB().Close()

			// Open the durable reference graph and register its cleanup.
			rg, err := NewRefGraph(ctx, store, []byte("gc/"))
			if err != nil {
				b.Fatal(err)
			}
			defer rg.Close()

			// Prepare and seed the initial ownership transition.
			old, next := make([]RefEdge, 722), make([]RefEdge, 722)
			for i := range old {
				object := "block-" + strconv.Itoa(i)
				old[i] = RefEdge{Subject: "owner-a", Object: object}
				next[i] = RefEdge{Subject: "owner-b", Object: object}
			}
			if err := rg.ApplyRefBatch(ctx, old, nil); err != nil {
				b.Fatal(err)
			}

			// Measure ownership transfer commits and allocations.
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				// Prepare the next ownership transition.
				adds, removes, err := rg.prepareRefBatch(ctx, next, old, true)
				if err != nil {
					b.Fatal(err)
				}

				// Apply ownership changes through the selected commit shape.
				if separate {
					err = rg.applyRefBatchChunk(ctx, adds, nil)
					if err == nil {
						err = rg.applyRefBatchChunk(ctx, nil, removes)
					}
				} else {
					_, _, err = rg.applyRefBatchSliceLocked(ctx, adds, removes)
				}
				if err != nil {
					b.Fatal(err)
				}

				// Reverse the ownership transition for the next sample.
				old, next = next, old
			}

			// Read the final ownership edge set outside the timed sample.
			b.StopTimer()
			found, err := rg.hasRefs(ctx, old)
			if err != nil {
				b.Fatal(err)
			}

			// Verify every final ownership edge remains present.
			for _, exists := range found {
				if !exists {
					b.Fatal("transfer lost an owner")
				}
			}
		})
	}
}
