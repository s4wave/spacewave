package resource_server

import (
	"context"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	"github.com/tidwall/btree"
)

// registryInitStream substitutes only the transport around a real Init request.
type registryInitStream struct {
	resource.SRPCResourceService_ResourceClientStream
}

// Context keeps the measured Init alive until its response is sent.
func (s *registryInitStream) Context() context.Context { return context.Background() }

// Recv supplies the first generation packet.
func (s *registryInitStream) Recv() (*resource.ResourceClientRequest, error) {
	return &resource.ResourceClientRequest{Body: &resource.ResourceClientRequest_Init{Init: &resource.ResourceClientInitRequest{}}}, nil
}

// Send ends the test transport after generation registration has completed.
func (s *registryInitStream) Send(*resource.ResourceClientResponse) error { return context.Canceled }

// BenchmarkGenerationRetirementInit measures real Init requests beside a large teardown.
func BenchmarkGenerationRetirementInit(b *testing.B) {
	// Retain samples across fixed-size teardowns, excluding tree construction from timing.
	var samples []int64
	var overlap int
	for b.Loop() {
		b.StopTimer()
		server := NewResourceServer(nil, "")
		client := &RemoteResourceClient{server: server, clientID: 1, rootResourceID: 1, ctx: context.Background(), resources: make(map[uint32]*trackedResource), children: make(map[uint32]*btree.Map[uint32, struct{}])}
		server.clients[1] = client
		server.clientHandleIDCtr = 1
		server.resourceIDCtr = 1
		server.routes[1] = client
		server.trackedCount = 1
		client.resources[1] = &trackedResource{mux: srpc.NewMux(), ownerClientID: 1}
		for range 50000 {
			if _, err := newResourceRPCContext(client, 1).AddResource(srpc.NewMux(), func() {}); err != nil {
				b.Fatal(err)
			}
		}

		// Start the teardown first, then measure the complete Init admission and cleanup.
		done := make(chan struct{})
		go func() { server.releaseClientGeneration(client); close(done) }()
		runtime.Gosched()
		b.StartTimer()
		for range 64 {
			start := time.Now()
			_ = server.ResourceClient(&registryInitStream{})
			samples = append(samples, time.Since(start).Nanoseconds())
			select {
			case <-done:
			default:
				overlap++
			}
		}
		<-done
	}

	// Report the latency distribution without imposing a loaded-host timing assertion.
	b.ReportMetric(float64(overlap)/float64(b.N), "overlapping-inits/op")
	slices.Sort(samples)
	b.ReportMetric(float64(samples[len(samples)/2]), "p50-init-ns")
	b.ReportMetric(float64(samples[(len(samples)-1)*99/100]), "p99-init-ns")
	b.ReportMetric(float64(samples[len(samples)-1]), "max-init-ns")
}
