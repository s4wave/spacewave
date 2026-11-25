package block_store_fillcache

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// readBenchBatch is the blocks one benchmark operation reads, about an Okra
// batch load or a warm-up slice.
const readBenchBatch = 64

// BenchmarkCacheRead reads batches of random blocks from a cache file of
// SPACEWAVE_FILL_BENCH_BLOCKS blocks of 1 to 16 KiB in each candidate row
// layout, with a transaction per block and one per batch. It reports the time
// per block and the file's size.
func BenchmarkCacheRead(b *testing.B) {
	n := harnessEnv("SPACEWAVE_FILL_BENCH_BLOCKS", 20000)
	le := logrus.NewEntry(logrus.New())
	current := filePragmas
	defer func() { filePragmas = current }()
	for _, layout := range []struct {
		name         string
		withoutRowID bool
		pageSize     int32
	}{
		{"without-rowid-4k", true, 4 << 10},
		{"without-rowid-16k", true, 16 << 10},
		{"rowid-4k", false, 4 << 10},
		{"rowid-16k", false, 16 << 10},
		{"rowid-64k", false, 64 << 10},
	} {
		// Fill one file in this layout.
		filePragmas = current
		filePragmas.WithoutRowID = layout.withoutRowID
		filePragmas.PageSize = layout.pageSize
		path := filepath.Join(b.TempDir(), layout.name+".db")
		refs := fillBenchFile(b, le, path, n)
		info, err := os.Stat(path)
		if err != nil {
			b.Fatal(err)
		}

		// Read it with a transaction per block and one per batch.
		for _, scoped := range []bool{false, true} {
			name := layout.name + "/tx-per-block"
			if scoped {
				name = layout.name + "/tx-per-batch"
			}
			b.Run(name, func(b *testing.B) {
				// Reopen the file for this case.
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
				b.ReportMetric(float64(info.Size())/float64(n), "file-B/block")
			})
		}
	}
}

// fillBenchFile writes n random blocks of 1 to 16 KiB into a new cache file at
// path, in the layout filePragmas names, and returns their refs.
func fillBenchFile(b *testing.B, le *logrus.Entry, path string, n int) []*block.BlockRef {
	// Open the file.
	c, err := Open(b.Context(), le, path, Options{LimitBytes: 1 << 40})
	if err != nil {
		b.Fatal(err)
	}

	// Queue the blocks, letting the writer catch up every 2,000.
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

	// Write the rest and close the file.
	if err := c.Wait(b.Context()); err != nil {
		b.Fatal(err)
	}
	if err := c.Close(); err != nil {
		b.Fatal(err)
	}
	return refs
}
