//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"net"
	"path/filepath"
	"runtime/trace"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	yamux "github.com/libp2p/go-yamux/v4"
	trace_capture "github.com/s4wave/spacewave/core/trace/capture"
	trace_service "github.com/s4wave/spacewave/core/trace/service"
	s4wave_trace "github.com/s4wave/spacewave/sdk/trace"
)

func TestCaptureDaemonRuntimeTraceWritesTrace(t *testing.T) {
	ctx := context.Background()
	traceClient := newDebugTraceTestClient(t)
	var out bytes.Buffer

	_, task := trace.NewTask(ctx, "debug-trace-test")
	trace.Log(ctx, "phase", "capture")
	task.End()

	byteCount, err := trace_capture.CaptureRuntimeTrace(
		ctx,
		traceClient,
		&out,
		trace_capture.RuntimeTraceArgs{
			Duration:    time.Millisecond,
			Label:       "debug-trace-test",
			StopTimeout: debugTraceStopTimeout,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if byteCount == 0 {
		t.Fatal("expected trace bytes")
	}
	if int64(out.Len()) != byteCount {
		t.Fatalf("expected %d bytes, got %d", byteCount, out.Len())
	}
}

func TestDefaultDebugTraceOutputPath(t *testing.T) {
	got := defaultDebugTraceOutputPath(time.Date(2026, 5, 11, 18, 20, 30, 0, time.UTC))
	want := ".tmp/spacewave-daemon-20260511-182030.trace"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestDefaultDebugCPUProfileOutputPath(t *testing.T) {
	got := defaultDebugCPUProfileOutputPath(time.Date(2026, 5, 11, 18, 20, 30, 0, time.UTC))
	want := ".tmp/spacewave-daemon-20260511-182030.pprof"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestDefaultDebugMemoryProfileOutputPath(t *testing.T) {
	got := defaultDebugMemoryProfileOutputPath(time.Date(2026, 5, 11, 18, 20, 30, 0, time.UTC), "allocs")
	want := ".tmp/spacewave-daemon-20260511-182030-allocs.pprof"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestConnectDebugTraceDaemonUsesSocketPathWithoutAutostart(t *testing.T) {
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	connA, connB := net.Pipe()
	t.Cleanup(func() {
		connA.Close()
		connB.Close()
	})

	sock := filepath.Join(t.TempDir(), "spacewave-debug.sock")
	var dialed string
	connectDaemonDial = func(ctx context.Context, sockPath string) (net.Conn, error) {
		dialed = sockPath
		return connA, nil
	}
	connectDaemonBuildClient = func(ctx context.Context, conn net.Conn) (*sdkClient, error) {
		return &sdkClient{conn: conn}, nil
	}
	connectDaemonStart = func(ctx context.Context, statePath string) error {
		t.Fatal("debug connection must not autostart daemon")
		return nil
	}

	client, err := connectDebugTraceDaemon(context.Background(), nil, t.TempDir(), sock)
	if err != nil {
		t.Fatalf("connect debug daemon: %v", err)
	}
	client.conn.Close()
	if dialed != sock {
		t.Fatalf("dialed %s, want %s", dialed, sock)
	}
}

func TestCaptureDaemonCPUProfileWritesProfile(t *testing.T) {
	ctx := context.Background()
	traceClient := newDebugTraceTestClient(t)
	var out bytes.Buffer

	byteCount, err := trace_capture.CaptureCPUProfile(
		ctx,
		traceClient,
		&out,
		trace_capture.CPUProfileArgs{
			Duration: 100 * time.Millisecond,
			Label:    "debug-cpu-profile-test",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if byteCount == 0 {
		t.Fatal("expected CPU profile bytes")
	}
	if int64(out.Len()) != byteCount {
		t.Fatalf("expected %d bytes, got %d", byteCount, out.Len())
	}
}

func TestCaptureDaemonMemoryProfileWritesProfile(t *testing.T) {
	ctx := context.Background()
	traceClient := newDebugTraceTestClient(t)
	var out bytes.Buffer

	byteCount, err := trace_capture.CaptureMemoryProfile(ctx, traceClient, &out, trace_capture.MemoryProfileArgs{Profile: "allocs"})
	if err != nil {
		t.Fatal(err)
	}
	if byteCount == 0 {
		t.Fatal("expected memory profile bytes")
	}
	if int64(out.Len()) != byteCount {
		t.Fatalf("expected %d bytes, got %d", byteCount, out.Len())
	}
}

func newDebugTraceTestClient(t *testing.T) s4wave_trace.SRPCTraceServiceClient {
	t.Helper()

	mux := srpc.NewMux()
	if err := s4wave_trace.SRPCRegisterTraceService(mux, trace_service.NewService()); err != nil {
		t.Fatal(err)
	}

	server := srpc.NewServer(mux)
	serverCtx, cancel := context.WithCancel(t.Context())
	clientConn, serverConn := net.Pipe()
	serverMux, err := srpc.NewMuxedConn(serverConn, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	client, err := srpc.NewClientWithConn(clientConn, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.AcceptMuxedConn(serverCtx, serverMux)
	}()
	t.Cleanup(func() {
		if err := serverMux.Close(); err != nil {
			t.Errorf("close server mux: %v", err)
		}
		acceptErr := <-serverErr
		cancel()
		if acceptErr != yamux.ErrSessionShutdown {
			t.Errorf("server accept: %v, want session shutdown", acceptErr)
		}
		if err := clientConn.Close(); err != nil {
			t.Errorf("close client connection: %v", err)
		}
	})
	return s4wave_trace.NewSRPCTraceServiceClient(client)
}
