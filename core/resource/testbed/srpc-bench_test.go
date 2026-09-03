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
	ctx := b.Context()
	stats := newBenchWireStats()
	clientConn, serverConn := dial(b)

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

	exec := func() error {
		return client.ExecCall(ctx, benchEmptyServiceID, benchEmptyMethodID, &srpc.RawMessage{}, &srpc.RawMessage{})
	}
	if err := exec(); err != nil {
		b.Fatal(err.Error())
	}

	stats.reset()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := exec(); err != nil {
			b.Fatal(err.Error())
		}
	}
	b.StopTimer()

	stats.report(b)

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
	type acceptResult struct {
		conn net.Conn
		err  error
	}
	accepted := make(chan acceptResult, 1)
	go func() {
		conn, err := ln.Accept()
		accepted <- acceptResult{conn, err}
	}()
	clientConn, err := net.Dial("unix", sockPath)
	if err != nil {
		ln.Close()
		b.Fatal(err.Error())
	}
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
	b.Helper()
	stats := newBenchWireStats()
	clientPipe, serverPipe := net.Pipe()

	clientMp, err := srpc.NewMuxedConn(&benchCountingConn{Conn: clientPipe, stats: stats}, true, nil)
	if err != nil {
		clientPipe.Close()
		serverPipe.Close()
		b.Fatal(err.Error())
	}
	srpcClient := srpc.NewClientWithMuxedConn(clientMp)

	rootMux := srpc.NewMux()
	if err := rootMux.Register(benchEmptyHandler{}); err != nil {
		b.Fatal(err.Error())
	}

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

	cleanup := func() {
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
	ctx := b.Context()
	rootSrpcClient, stats, cleanup := setupBenchRootResourceClient(ctx, b)
	defer cleanup()

	exec := func() error {
		return rootSrpcClient.ExecCall(ctx, benchEmptyServiceID, benchEmptyMethodID, &srpc.RawMessage{}, &srpc.RawMessage{})
	}
	if err := exec(); err != nil {
		b.Fatal(err.Error())
	}

	stats.reset()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := exec(); err != nil {
			b.Fatal(err.Error())
		}
	}
	b.StopTimer()

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
		_, found, err := tx.GetObject(ctx, key)
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
	b.Helper()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		b.Fatal(err.Error())
	}
	resClient, clientCleanup := resource_testbed.SetupResourceClient(ctx, b, tb)

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
	ctx := b.Context()
	_, engine, cleanup := setupBenchWorldEngine(ctx, b)
	defer cleanup()

	if _, err := engine.GetSeqno(ctx); err != nil {
		b.Fatal(err.Error())
	}

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
	ctx := b.Context()
	_, engine, cleanup := setupBenchWorldEngine(ctx, b)
	defer cleanup()

	warmupTx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		b.Fatal(err.Error())
	}
	if err := warmupTx.Discard(ctx); err != nil {
		warmupTx.Release()
		b.Fatal(err.Error())
	}
	warmupTx.Release()

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
	ctx := b.Context()
	_, engine, cleanup := setupBenchWorldEngine(ctx, b)
	defer cleanup()

	mutateOnce := func(pred string) error {
		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		if err := tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("bench-subj", pred, "bench-obj", "")); err != nil {
			tx.Discard(ctx)
			tx.Release()
			return err
		}
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

	directMutateOnce := func(pred string) error {
		tx, err := tb.Engine.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		if err := tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("bench-subj", pred, "bench-obj", "")); err != nil {
			tx.Discard()
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return nil
	}
	if err := directMutateOnce("bench-pred-warmup"); err != nil {
		b.Fatal(err.Error())
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := directMutateOnce("bench-pred-" + strconv.Itoa(i)); err != nil {
			b.Fatal(err.Error())
		}
	}
	b.StopTimer()
}
