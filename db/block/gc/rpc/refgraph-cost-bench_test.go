package block_gc_rpc_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_gc_rpc "github.com/s4wave/spacewave/db/block/gc/rpc"
	block_gc_rpc_client "github.com/s4wave/spacewave/db/block/gc/rpc/client"
	block_gc_rpc_server "github.com/s4wave/spacewave/db/block/gc/rpc/server"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// BenchmarkRPCRefGraphRemovalCostByStoreSize measures the bounded removal
// workload through the real in-process SRPC client/server testbed.
func BenchmarkRPCRefGraphRemovalCostByStoreSize(b *testing.B) {
	ctx := context.Background()
	for _, edgeCount := range []int{1, 8, 16, 32, 64, 96, 128} {
		b.Run("edges="+strconv.Itoa(edgeCount), func(b *testing.B) {
			var total time.Duration
			for iteration := range b.N {
				b.StopTimer()
				rg, cleanup := newBenchmarkRPCRefGraph(b, ctx, edgeCount, iteration)
				b.StartTimer()
				start := time.Now()
				if err := rg.ApplyRefBatch(ctx, nil, benchmarkRemoves(edgeCount, iteration)); err != nil {
					b.Fatal(err)
				}
				total += time.Since(start)
				b.StopTimer()
				cleanup()
			}
			if b.N != 0 {
				b.ReportMetric(float64(total.Nanoseconds())/float64(b.N), "transport_total_ns/op")
			}
			b.ReportMetric(float64(edgeCount), "removal_edges/op")
		})
	}
}

func newBenchmarkRPCRefGraph(
	b *testing.B,
	ctx context.Context,
	edgeCount, iteration int,
) (block_gc.RefGraphOps, func()) {
	b.Helper()
	store := store_kvtx_inmem.NewStore()
	serverGraph, err := block_gc.NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		b.Fatal(err)
	}
	mux := srpc.NewMux()
	if err := block_gc_rpc.SRPCRegisterRefGraph(mux, block_gc_rpc_server.NewRefGraph(serverGraph)); err != nil {
		serverGraph.Close()
		b.Fatal(err)
	}
	server := srpc.NewServer(mux)
	openStream := srpc.NewServerPipe(server)
	client := srpc.NewClient(openStream)
	rg := block_gc_rpc_client.NewRefGraph(block_gc_rpc.NewSRPCRefGraphClient(client))
	removes := benchmarkRemoves(edgeCount, iteration)
	for i, edge := range removes {
		if i%2 == 0 {
			if err := rg.AddRef(ctx, edge.Subject, edge.Object); err != nil {
				serverGraph.Close()
				b.Fatal(err)
			}
		}
	}
	return rg, func() {
		if err := serverGraph.Close(); err != nil {
			b.Error(err)
		}
	}
}

func benchmarkRemoves(edgeCount, iteration int) []block_gc.RefEdge {
	removes := make([]block_gc.RefEdge, edgeCount)
	prefix := "rpc-cost/" + strconv.Itoa(iteration) + "/"
	for i := range removes {
		removes[i] = block_gc.RefEdge{
			Subject: prefix + "subject/" + strconv.Itoa(i),
			Object:  prefix + "object/" + strconv.Itoa(i),
		}
	}
	return removes
}
