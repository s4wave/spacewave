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
	ctx := b.Context()
	store, err := store_kvtx_bolt.Open(filepath.Join(b.TempDir(), "refs.db"), 0o600, nil, []byte("test"))
	if err != nil {
		b.Fatal(err)
	}
	defer store.GetDB().Close()
	rg, err := NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		b.Fatal(err)
	}
	defer rg.Close()
	edges := make([]RefEdge, 4096)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := range b.N {
		b.StopTimer()
		for i := range edges {
			edges[i] = RefEdge{Subject: "batch-" + strconv.Itoa(iteration), Object: "block-" + strconv.Itoa(i)}
		}
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
		name := "combined"
		if separate {
			name = "separate"
		}
		b.Run(name, func(b *testing.B) {
			ctx := b.Context()
			store, err := store_kvtx_bolt.Open(filepath.Join(b.TempDir(), "refs.db"), 0o600, nil, []byte("test"))
			if err != nil {
				b.Fatal(err)
			}
			defer store.GetDB().Close()
			rg, err := NewRefGraph(ctx, store, []byte("gc/"))
			if err != nil {
				b.Fatal(err)
			}
			defer rg.Close()
			old, next := make([]RefEdge, 722), make([]RefEdge, 722)
			for i := range old {
				object := "block-" + strconv.Itoa(i)
				old[i] = RefEdge{Subject: "owner-a", Object: object}
				next[i] = RefEdge{Subject: "owner-b", Object: object}
			}
			if err := rg.ApplyRefBatch(ctx, old, nil); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				adds, removes, err := rg.prepareRefBatch(ctx, next, old, true)
				if err != nil {
					b.Fatal(err)
				}
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
				old, next = next, old
			}
			b.StopTimer()
			found, err := rg.hasRefs(ctx, old)
			if err != nil {
				b.Fatal(err)
			}
			for _, exists := range found {
				if !exists {
					b.Fatal("transfer lost an owner")
				}
			}
		})
	}
}
