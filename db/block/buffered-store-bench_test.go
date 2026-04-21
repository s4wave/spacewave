package block

import (
	"context"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/net/hash"
)

func BenchmarkBufferedStoreDrainMatrix(b *testing.B) {
	ctx := context.Background()
	payloads := buildBufferedStoreBenchmarkPayloads(256)

	for _, drainBatchEntries := range []int{16, 64, 256} {
		b.Run("drain-batch-"+strconv.Itoa(drainBatchEntries), func(b *testing.B) {
			settings := &BufferedStoreSettings{
				DrainBatchEntries: drainBatchEntries,
			}
			var totalBatchCalls int
			var totalBatchEntries int

			b.ReportAllocs()

			for b.Loop() {
				inner := newCountStore(hash.HashType_HashType_BLAKE3)
				store := NewBufferedStoreWithSettings(ctx, inner, settings)
				for _, payload := range payloads {
					if _, _, err := store.PutBlock(ctx, payload, nil); err != nil {
						b.Fatal(err)
					}
				}
				if err := store.Flush(ctx); err != nil {
					b.Fatal(err)
				}

				inner.mtx.Lock()
				totalBatchCalls += inner.batchCalls
				for _, sz := range inner.batchSizes {
					totalBatchEntries += sz
				}
				inner.mtx.Unlock()
			}

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
