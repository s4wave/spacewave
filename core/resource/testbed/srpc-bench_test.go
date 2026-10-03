//go:build !js

package resource_testbed_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// benchEmptyServiceID and benchEmptyMethodID identify a unary service whose
// request and response messages are both zero bytes. It isolates SRPC
// protocol cost from any application payload handling.
const (
	benchEmptyServiceID = "testbed.bench.EmptyService"
	benchEmptyMethodID  = "Empty"
)

// benchGraphEndpointKeys are the object keys every graph-write benchmark
// requires to exist before it mutates; SetGraphQuad resolves subject and
// object through object lookups and fails with "object not found" otherwise.
var benchGraphEndpointKeys = []string{"bench-subj", "bench-obj"}

// benchEmptyHandler serves one empty unary method.
type benchEmptyHandler struct{}

func (benchEmptyHandler) GetServiceID() string   { return benchEmptyServiceID }
func (benchEmptyHandler) GetMethodIDs() []string { return []string{benchEmptyMethodID} }

func (benchEmptyHandler) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	if serviceID != benchEmptyServiceID || methodID != benchEmptyMethodID {
		return false, nil
	}
	in := &srpc.RawMessage{}
	if err := strm.MsgRecv(in); err != nil {
		return true, err
	}
	return true, strm.MsgSend(&srpc.RawMessage{})
}

var _ srpc.Handler = benchEmptyHandler{}

// benchWireStats counts frames (writes) and bytes observed on a connection.
// Totals are bidirectional: both muxed-conn halves wrap the same stats, so
// reported per-op values include client and server traffic combined.
type benchWireStats struct {
	frames atomic.Int64
	bytes  atomic.Int64
}

func newBenchWireStats() *benchWireStats { return &benchWireStats{} }

func (s *benchWireStats) addFrame(n int) {
	s.frames.Add(1)
	s.bytes.Add(int64(n))
}

func (s *benchWireStats) reset() {
	s.frames.Store(0)
	s.bytes.Store(0)
}

func (s *benchWireStats) report(b *testing.B) {
	n := float64(b.N)
	b.ReportMetric(float64(s.frames.Load())/n, "frames/op")
	b.ReportMetric(float64(s.bytes.Load())/n, "wire-bytes/op")
}

type benchCountingConn struct {
	net.Conn
	stats *benchWireStats
}

func (c *benchCountingConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	c.stats.addFrame(n)
	return n, err
}

func benchmarkSRPCUnaryEmpty(b *testing.B, dial func(b *testing.B) (net.Conn, net.Conn)) {
	// Dial the benchmark connection and initialize wire statistics.
	ctx := b.Context()
	stats := newBenchWireStats()
	clientConn, serverConn := dial(b)

	// Connect counted client and server SRPC multiplexers.
	clientMp, err := srpc.NewMuxedConn(&benchCountingConn{Conn: clientConn, stats: stats}, true, nil)
	if err != nil {
		clientConn.Close()
		serverConn.Close()
		b.Fatal(err.Error())
	}
	serverMp, err := srpc.NewMuxedConn(&benchCountingConn{Conn: serverConn, stats: stats}, false, nil)
	if err != nil {
		clientConn.Close()
		serverConn.Close()
		b.Fatal(err.Error())
	}
	client := srpc.NewClientWithMuxedConn(clientMp)

	// Serve the empty unary method on the benchmark connection.
	mux := srpc.NewMux()
	if err := mux.Register(benchEmptyHandler{}); err != nil {
		b.Fatal(err.Error())
	}
	server := srpc.NewServer(mux)
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		_ = server.AcceptMuxedConn(ctx, serverMp)
	}()

	// Warm the empty unary call before measuring it.
	exec := func() error {
		return client.ExecCall(ctx, benchEmptyServiceID, benchEmptyMethodID, &srpc.RawMessage{}, &srpc.RawMessage{})
	}
	if err := exec(); err != nil {
		b.Fatal(err.Error())
	}

	// Measure repeated empty unary calls and their allocations.
	stats.reset()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := exec(); err != nil {
			b.Fatal(err.Error())
		}
	}
	b.StopTimer()

	// Report the empty unary call wire cost.
	stats.report(b)

	// Close both SRPC multiplexers and wait for server completion.
	clientMp.Close()
	serverMp.Close()
	select {
	case <-acceptDone:
	case <-time.After(2 * time.Second):
	}
}

func BenchmarkSRPCUnaryEmptyNetPipe(b *testing.B) {
	benchmarkSRPCUnaryEmpty(b, func(b *testing.B) (net.Conn, net.Conn) {
		return net.Pipe()
	})
}

func BenchmarkSRPCUnaryEmptyUnixSocket(b *testing.B) {
	benchmarkSRPCUnaryEmpty(b, dialUnixSocketPair)
}

func dialUnixSocketPair(b *testing.B) (net.Conn, net.Conn) {
	// macOS rejects unix socket paths over 104 bytes; keep the directory short.
	dir, err := os.MkdirTemp("", "srpc-bench")
	if err != nil {
		b.Fatal(err.Error())
	}
	sockPath := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		os.RemoveAll(dir)
		b.Fatal(err.Error())
	}
	defer ln.Close()
	defer os.RemoveAll(dir)

	// Accept one Unix socket peer for the benchmark connection.
	type acceptResult struct {
		conn net.Conn
		err  error
	}
	accepted := make(chan acceptResult, 1)
	go func() {
		conn, err := ln.Accept()
		accepted <- acceptResult{conn, err}
	}()

	// Connect the client to the benchmark Unix socket listener.
	clientConn, err := net.Dial("unix", sockPath)
	if err != nil {
		ln.Close()
		b.Fatal(err.Error())
	}

	// Require the accepted Unix socket peer to be usable.
	res := <-accepted
	if res.err != nil {
		clientConn.Close()
		ln.Close()
		b.Fatal(res.err.Error())
	}
	return clientConn, res.conn
}

// setupBenchRootResourceClient wires a resource client to a ResourceServer
// over a counted net.Pipe. The root mux serves only the empty unary handler,
// so the benchmark measures the resource indirection itself.
func setupBenchRootResourceClient(ctx context.Context, b *testing.B) (srpc.Client, *benchWireStats, func()) {
	// Prepare the counted in-memory resource connection.
	b.Helper()
	stats := newBenchWireStats()
	clientPipe, serverPipe := net.Pipe()

	// Connect the counted pipe to the SRPC client.
	clientMp, err := srpc.NewMuxedConn(&benchCountingConn{Conn: clientPipe, stats: stats}, true, nil)
	if err != nil {
		clientPipe.Close()
		serverPipe.Close()
		b.Fatal(err.Error())
	}
	srpcClient := srpc.NewClientWithMuxedConn(clientMp)

	// Register the empty unary method on the root resource mux.
	rootMux := srpc.NewMux()
	if err := rootMux.Register(benchEmptyHandler{}); err != nil {
		b.Fatal(err.Error())
	}

	// Serve the root resource through a counted SRPC connection.
	wireMux := srpc.NewMux()
	server := srpc.NewServer(wireMux)
	resourceServer := resource_server.NewResourceServer(rootMux)
	if err := resourceServer.Register(wireMux); err != nil {
		b.Fatal(err.Error())
	}
	serverMp, err := srpc.NewMuxedConn(&benchCountingConn{Conn: serverPipe, stats: stats}, false, nil)
	if err != nil {
		clientPipe.Close()
		serverPipe.Close()
		b.Fatal(err.Error())
	}
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		_ = server.AcceptMuxedConn(ctx, serverMp)
	}()

	// Access the root resource through the resource client.
	resClient, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(srpcClient))
	if err != nil {
		clientPipe.Close()
		serverPipe.Close()
		b.Fatal(err.Error())
	}
	rootRef := resClient.AccessRootResource()
	rootSrpcClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		resClient.Release()
		clientPipe.Close()
		serverPipe.Close()
		b.Fatal(err.Error())
	}

	// Provide cleanup for the resource references and SRPC connection.
	cleanup := func() {
		// Release the root resource and close the SRPC connection.
		rootRef.Release()
		resClient.Release()
		clientMp.Close()
		serverMp.Close()
		select {
		case <-acceptDone:
		case <-time.After(2 * time.Second):
		}
	}
	return rootSrpcClient, stats, cleanup
}

func BenchmarkRootResourceUnaryEmptyNetPipe(b *testing.B) {
	// Connect the benchmark to the empty root resource method.
	ctx := b.Context()
	rootSrpcClient, stats, cleanup := setupBenchRootResourceClient(ctx, b)
	defer cleanup()

	// Warm the root resource unary call before measuring it.
	exec := func() error {
		return rootSrpcClient.ExecCall(ctx, benchEmptyServiceID, benchEmptyMethodID, &srpc.RawMessage{}, &srpc.RawMessage{})
	}
	if err := exec(); err != nil {
		b.Fatal(err.Error())
	}

	// Measure repeated root resource unary calls and allocations.
	stats.reset()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := exec(); err != nil {
			b.Fatal(err.Error())
		}
	}
	b.StopTimer()

	// Report the root resource unary call wire cost.
	stats.report(b)
}

// benchEndpointTx is the object-lookup surface of an open transaction.
type benchEndpointTx interface {
	GetObject(ctx context.Context, key string) (world.ObjectState, bool, error)
}

// requireGraphEndpoints reads each graph endpoint object through a fresh
// read transaction on the same engine the writes will use. This proves the
// cross-transaction visibility SetGraphQuad depends on before any timed work.
func requireGraphEndpoints(ctx context.Context, b *testing.B, open func() (benchEndpointTx, func())) {
	b.Helper()
	for _, key := range benchGraphEndpointKeys {
		tx, discard := open()
		objectState, found, err := tx.GetObject(ctx, key)
		world.ReleaseObjectState(objectState)
		if err == nil && !found {
			err = world.ErrObjectNotFound
		}
		if err != nil {
			discard()
			b.Fatalf("graph endpoint %q not readable after setup commit: %v", key, err)
		}
		discard()
	}
}

// setupBenchWorldEngine creates a world testbed and an SDK engine connected
// over the testbed resource client.
func setupBenchWorldEngine(ctx context.Context, b *testing.B) (*world_testbed.Testbed, *s4wave_world.Engine, func()) {
	// Create a World testbed and its resource connection.
	b.Helper()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		b.Fatal(err.Error())
	}
	resClient, clientCleanup := resource_testbed.SetupResourceClient(ctx, b, tb)

	// Create the benchmark World through the root RPC service.
	rootRef := resClient.AccessRootResource()
	srpcClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		clientCleanup()
		tb.Release()
		b.Fatal(err.Error())
	}
	testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
	createWorldResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
	if err != nil {
		rootRef.Release()
		clientCleanup()
		tb.Release()
		b.Fatal(err.Error())
	}

	// Wrap the created World engine with the SDK.
	engineRef := resClient.CreateResourceReference(createWorldResp.ResourceId)
	engine, err := s4wave_world.NewEngine(resClient, engineRef)
	if err != nil {
		rootRef.Release()
		clientCleanup()
		tb.Release()
		b.Fatal(err.Error())
	}

	// SetGraphQuad requires the subject and object to be existing object keys.
	setupTx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		b.Fatal(err.Error())
	}
	for _, key := range benchGraphEndpointKeys {
		objState, err := setupTx.CreateObject(ctx, key, nil)
		if err != nil {
			setupTx.Discard(ctx)
			b.Fatal(err.Error())
		}
		world.ReleaseObjectState(objState)
	}
	if err := setupTx.Commit(ctx); err != nil {
		b.Fatal(err.Error())
	}
	requireGraphEndpoints(ctx, b, func() (benchEndpointTx, func()) {
		readTx, err := engine.NewTransaction(ctx, false)
		if err != nil {
			b.Fatal(err.Error())
		}
		return readTx, func() { _ = readTx.Discard(ctx) }
	})

	// Provide cleanup for the SDK engine and testbed resources.
	cleanup := func() {
		engine.Release()
		rootRef.Release()
		clientCleanup()
		tb.Release()
	}
	return tb, engine, cleanup
}

// BenchmarkWorldGetSeqnoSrpcNetPipe measures one read-only RPC through the
// world engine resource: pure SRPC round trips plus a trivial state read.
func BenchmarkWorldGetSeqnoSrpcNetPipe(b *testing.B) {
	// Create the SDK World engine for sequence read measurements.
	ctx := b.Context()
	_, engine, cleanup := setupBenchWorldEngine(ctx, b)
	defer cleanup()

	// Warm the World sequence read before measuring it.
	if _, err := engine.GetSeqno(ctx); err != nil {
		b.Fatal(err.Error())
	}

	// Measure repeated World sequence reads over SRPC.
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := engine.GetSeqno(ctx); err != nil {
			b.Fatal(err.Error())
		}
	}
	b.StopTimer()
}

// BenchmarkWorldTxLifecycleSrpcNetPipe measures NewTransaction plus Discard
// over SRPC: the transaction lifecycle cost without any mutation or commit.
func BenchmarkWorldTxLifecycleSrpcNetPipe(b *testing.B) {
	// Create the SDK World engine for transaction lifecycle measurements.
	ctx := b.Context()
	_, engine, cleanup := setupBenchWorldEngine(ctx, b)
	defer cleanup()

	// Warm the World transaction lifecycle before measuring it.
	warmupTx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		b.Fatal(err.Error())
	}
	if err := warmupTx.Discard(ctx); err != nil {
		warmupTx.Release()
		b.Fatal(err.Error())
	}
	warmupTx.Release()

	// Measure World transaction creation and discard over SRPC.
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			b.Fatal(err.Error())
		}
		if err := tx.Discard(ctx); err != nil {
			tx.Release()
			b.Fatal(err.Error())
		}
		tx.Release()
	}
	b.StopTimer()
}

// BenchmarkWorldMutationCommitSrpcNetPipe measures SetGraphQuad plus Commit
// over SRPC against the in-memory testbed volume.
func BenchmarkWorldMutationCommitSrpcNetPipe(b *testing.B) {
	// Create the SDK World engine for mutation measurements.
	ctx := b.Context()
	_, engine, cleanup := setupBenchWorldEngine(ctx, b)
	defer cleanup()

	// Define one World graph mutation and commit over SRPC.
	mutateOnce := func(pred string) error {
		// Open a write transaction for the World graph mutation.
		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			return err
		}

		// Write the graph quad into the SDK transaction.
		if err := tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("bench-subj", pred, "bench-obj", "")); err != nil {
			tx.Discard(ctx)
			tx.Release()
			return err
		}

		// Commit the World graph mutation and release its transaction.
		if err := tx.Commit(ctx); err != nil {
			tx.Release()
			return err
		}
		tx.Release()
		return nil
	}

	// The warmup predicate sits outside the timed strconv.Itoa range.
	if err := mutateOnce("bench-pred-warmup"); err != nil {
		b.Fatal(err.Error())
	}

	// Measure distinct World graph mutations and commits over SRPC.
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := mutateOnce("bench-pred-" + strconv.Itoa(i)); err != nil {
			b.Fatal(err.Error())
		}
	}
	b.StopTimer()
}

// BenchmarkWorldMutationCommitDirect measures the same SetGraphQuad plus
// Commit against the engine without SRPC, separating persistence cost from
// protocol overhead.
func BenchmarkWorldMutationCommitDirect(b *testing.B) {
	// Create a direct World testbed for mutation measurements.
	ctx := b.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		b.Fatal(err.Error())
	}
	defer tb.Release()

	// Create the graph endpoints once; see the SRPC variant for the per-op
	// predicate note.
	setupTx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		b.Fatal(err.Error())
	}
	for _, key := range benchGraphEndpointKeys {
		objState, err := setupTx.CreateObject(ctx, key, nil)
		if err != nil {
			setupTx.Discard()
			b.Fatal(err.Error())
		}
		world.ReleaseObjectState(objState)
	}
	if err := setupTx.Commit(ctx); err != nil {
		b.Fatal(err.Error())
	}
	requireGraphEndpoints(ctx, b, func() (benchEndpointTx, func()) {
		readTx, err := tb.Engine.NewTransaction(ctx, false)
		if err != nil {
			b.Fatal(err.Error())
		}
		return readTx, readTx.Discard
	})

	// Define one direct World graph mutation and commit.
	directMutateOnce := func(pred string) error {
		// Open a direct World write transaction.
		tx, err := tb.Engine.NewTransaction(ctx, true)
		if err != nil {
			return err
		}

		// Write the graph quad into the direct transaction.
		if err := tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("bench-subj", pred, "bench-obj", "")); err != nil {
			tx.Discard()
			return err
		}

		// Commit the direct World graph mutation.
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return nil
	}

	// Warm the direct World mutation before measuring it.
	if err := directMutateOnce("bench-pred-warmup"); err != nil {
		b.Fatal(err.Error())
	}

	// Measure distinct direct World graph mutations and commits.
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := directMutateOnce("bench-pred-" + strconv.Itoa(i)); err != nil {
			b.Fatal(err.Error())
		}
	}
	b.StopTimer()
}
