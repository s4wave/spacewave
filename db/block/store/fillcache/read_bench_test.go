package block_store_fillcache

import (
	"math/rand/v2"
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// readBenchBatch is the blocks one benchmark operation reads, about an Okra
// batch load or a warm-up slice.
const readBenchBatch = 64

// BenchmarkCacheRead reads batches of random blocks from a cache file of
// SPACEWAVE_FILL_BENCH_BLOCKS blocks of 1 to 16 KiB, four ways: a transaction
// per block or one per batch, each with the file read through read calls or
// a mapping. It reports the time per block.
func BenchmarkCacheRead(b *testing.B) {
	// Fill one cache file for every case.
	n := harnessEnv("SPACEWAVE_FILL_BENCH_BLOCKS", 20000)
	path := filepath.Join(b.TempDir(), "cache.db")
	le := logrus.NewEntry(logrus.New())
	c, err := Open(b.Context(), le, path, Options{LimitBytes: 1 << 40})
	if err != nil {
		b.Fatal(err)
	}
	rng := rand.New(rand.NewPCG(3, 4))
	refs := make([]*block.BlockRef, 0, n)
	for range n {
		data := make([]byte, 1<<10+rng.IntN(15<<10))
		for j := range data {
			data[j] = byte(rng.Uint32())
		}
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			b.Fatal(err)
		}
		refs = append(refs, ref)
		c.enqueue(&block.PutBatchEntry{Ref: ref, Data: data})
		if len(refs)%2000 == 0 {
			if err := c.Wait(b.Context()); err != nil {
				b.Fatal(err)
			}
		}
	}
	if err := c.Wait(b.Context()); err != nil {
		b.Fatal(err)
	}
	if err := c.Close(); err != nil {
		b.Fatal(err)
	}

	// Run each case on the file reopened with its mapping.
	for _, mmap := range []struct {
		name  string
		bytes int64
	}{{"read", 0}, {"mmap", 1 << 40}} {
		for _, scoped := range []bool{false, true} {
			name := mmap.name + "/tx-per-block"
			if scoped {
				name = mmap.name + "/tx-per-batch"
			}
			b.Run(name, func(b *testing.B) {
				// Reopen the file with this case's mapping.
				mmapBytes = mmap.bytes
				defer func() { mmapBytes = 0 }()
				c, err := Open(b.Context(), le, path, Options{LimitBytes: 1 << 40})
				if err != nil {
					b.Fatal(err)
				}
				defer c.Close()

				// Read a batch of random blocks per operation.
				rng := rand.New(rand.NewPCG(5, 6))
				var store block.StoreOps = c.blocks
				for b.Loop() {
					release := func() {}
					if scoped {
						store, release, err = c.blocks.BeginReadOperation(b.Context())
						if err != nil {
							b.Fatal(err)
						}
					}
					for range readBenchBatch {
						if _, found, err := store.GetBlock(b.Context(), refs[rng.IntN(len(refs))]); err != nil || !found {
							b.Fatalf("read = %v, %v", found, err)
						}
					}
					release()
				}
				b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*readBenchBatch), "ns/block")
			})
		}
	}
}
