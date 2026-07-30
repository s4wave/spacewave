package block_gc_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
)

func BenchmarkRefGraphApplyRefBatch(b *testing.B) {
	ctx := context.Background()
	for _, edgeCount := range []int{64, 1024, 4096*2 + 17} {
		b.Run("edges="+strconv.Itoa(edgeCount), func(b *testing.B) {
			rg := newBenchRefGraph(b, ctx)

			b.ReportAllocs()
			var readCount, readBytes uint64
			b.ResetTimer()
			for i := range b.N {
				b.StopTimer()
				adds := makeRefGraphBenchEdges(i, edgeCount)
				b.StartTimer()
				opCtx, counter := block.WithReadCounter(ctx)
				if err := rg.ApplyRefBatch(opCtx, adds, nil); err != nil {
					b.Fatal(err)
				}
				snapshot := counter.Snapshot()
				readCount += snapshot.BlockReadCount
				readBytes += snapshot.BlockReadBytes
			}
			b.ReportMetric(float64(edgeCount), "ref_edges/op")
			b.ReportMetric(float64(refGraphBenchTransactions(edgeCount)), "cayley_transactions/op")
			reportRefGraphBenchBlockReads(b, readCount, readBytes)
		})
	}
}

func newBenchRefGraph(b *testing.B, ctx context.Context) *block_gc.RefGraph {
	b.Helper()

	store := block_mock.NewMockStore(0)
	_, rootCursor := block.NewTransaction(store, nil, nil, nil)
	rootCursor.SetBlock(kvtx_block.NewKeyValueStore(kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA), true)
	ktx, err := kvtx_block.BuildKvTransaction(ctx, rootCursor, true)
	if err != nil {
		b.Fatal(err)
	}
	rg, err := block_gc.NewRefGraph(ctx, kvtx.NewTxStore(ktx), []byte("gc/"))
	if err != nil {
		ktx.Discard()
		b.Fatal(err)
	}
	b.Cleanup(func() {
		rg.Close()
		ktx.Discard()
	})
	return rg
}

func makeRefGraphBenchEdges(iteration, count int) []block_gc.RefEdge {
	edges := make([]block_gc.RefEdge, count)
	prefix := "bench/refgraph/" + strconv.Itoa(iteration) + "/"
	for i := range edges {
		suffix := strconv.Itoa(i)
		edges[i] = block_gc.RefEdge{
			Subject: prefix + "subject/" + suffix,
			Object:  prefix + "object/" + suffix,
		}
	}
	return edges
}

func refGraphBenchTransactions(edges int) int {
	const refGraphApplyBatchLimit = 512
	if edges == 0 {
		return 0
	}
	return (edges + refGraphApplyBatchLimit - 1) / refGraphApplyBatchLimit
}

func reportRefGraphBenchBlockReads(b *testing.B, readCount, readBytes uint64) {
	if b.N == 0 {
		return
	}
	denom := float64(b.N)
	b.ReportMetric(float64(readCount)/denom, "block-reads/op")
	b.ReportMetric(float64(readBytes)/denom, "block-read-bytes/op")
}
