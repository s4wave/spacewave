package block

import (
	"context"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/net/hash"
)

func BenchmarkBufferedStoreDrainMatrix(b *testing.B) {
	ctx := context.Background()
	payloads := buildBufferedStoreBenchmarkPayloads(256)

	for _, drainBatchEntries := range []int{16, 64, 256} {
		b.Run("drain-batch-"+strconv.Itoa(drainBatchEntries), func(b *testing.B) {
			// Configure the buffered-store drain batch and aggregate metrics.
			settings := &BufferedStoreSettings{
				DrainBatchEntries: drainBatchEntries,
			}
			var totalBatchCalls int
			var totalBatchEntries int
			b.ReportAllocs()

			// Drain each payload set and collect backing batch totals.
			for b.Loop() {
				inner := newCountStore(hash.HashType_HashType_BLAKE3)
				store := NewBufferedStoreWithSettings(ctx, inner, settings)
				for _, payload := range payloads {
					if _, _, err := store.PutBlock(ctx, payload, nil); err != nil {
						b.Fatal(err)
					}
				}
				if _, err := store.Sync(ctx); err != nil {
					b.Fatal(err)
				}

				inner.mtx.Lock()
				totalBatchCalls += inner.batchCalls
				for _, sz := range inner.batchSizes {
					totalBatchEntries += sz
				}
				inner.mtx.Unlock()
			}

			// Report the measured batch count and average batch size.
			if totalBatchCalls > 0 {
				b.ReportMetric(float64(totalBatchCalls)/float64(b.N), "batches/op")
				b.ReportMetric(float64(totalBatchEntries)/float64(totalBatchCalls), "entries/batch")
			}
		})
	}
}

func buildBufferedStoreBenchmarkPayloads(count int) [][]byte {
	payloads := make([][]byte, count)
	for i := range count {
		payloads[i] = []byte("buffered-store-bench-" + strconv.Itoa(i))
	}
	return payloads
}

// BenchmarkPendingStorePinRoot times PinRoot and its release on the root of a
// kept tree, each of which holds the PendingStore's lock exclusive, and the
// wait of a Put racing them, at two tree sizes.
func BenchmarkPendingStorePinRoot(b *testing.B) {
	ctx := context.Background()
	for _, n := range []int{1000, 10000} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			// Keep a tree of n blocks, four children per block.
			ps := NewPendingStore(newCountStore(hash.HashType_HashType_BLAKE3), 64<<20)
			entries := make([]*PutBatchEntry, n)
			for i := n - 1; i >= 0; i-- {
				var refs []*BlockRef
				for c := 4*i + 1; c <= 4*i+4 && c < n; c++ {
					refs = append(refs, entries[c].Ref)
				}
				data := []byte("pin " + strconv.Itoa(i))
				ref, err := BuildBlockRef(data, nil)
				if err != nil {
					b.Fatal(err)
				}
				entries[i] = &PutBatchEntry{Ref: ref, Data: data, Refs: refs, RefsKnown: true}
			}
			if _, err := ps.PutBlockBatch(ctx, entries); err != nil {
				b.Fatal(err)
			}
			root := entries[0].Ref

			// Put fresh blocks while the pins run, timing each.
			stop := make(chan struct{})
			waits := make(chan []time.Duration)
			go func() {
				var out []time.Duration
				opts := &PutOpts{RefsKnown: true}
				for i := 0; ; i++ {
					select {
					case <-stop:
						waits <- out
						return
					default:
					}
					data := []byte("put " + strconv.Itoa(i))
					start := time.Now()
					if _, _, err := ps.PutBlock(ctx, data, opts); err != nil {
						panic(err)
					}
					out = append(out, time.Since(start))
				}
			}()

			// Pin and release the root, timing each.
			var pinMax, releaseMax time.Duration
			for b.Loop() {
				start := time.Now()
				release, err := ps.PinRoot(ctx, root)
				if err != nil {
					b.Fatal(err)
				}
				pinned := time.Now()
				release()
				pinMax = max(pinMax, pinned.Sub(start))
				releaseMax = max(releaseMax, time.Since(pinned))
			}
			close(stop)
			out := <-waits
			slices.Sort(out)
			b.ReportMetric(float64(pinMax.Microseconds()), "pin-max-us")
			b.ReportMetric(float64(releaseMax.Microseconds()), "release-max-us")
			if len(out) != 0 {
				b.ReportMetric(float64(out[len(out)*99/100].Microseconds()), "put-p99-us")
				b.ReportMetric(float64(out[len(out)-1].Microseconds()), "put-max-us")
			}
		})
	}
}
