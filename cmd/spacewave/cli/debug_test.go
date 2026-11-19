//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"net"
	"path/filepath"
	"runtime/trace"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/google/pprof/profile"
	yamux "github.com/libp2p/go-yamux/v5"
	"github.com/pkg/errors"
	trace_capture "github.com/s4wave/spacewave/core/trace/capture"
	trace_service "github.com/s4wave/spacewave/core/trace/service"
	s4wave_trace "github.com/s4wave/spacewave/sdk/trace"
)

func TestCaptureDaemonRuntimeTraceWritesTrace(t *testing.T) {
	// Build a trace client and output buffer.
	ctx := context.Background()
	traceClient := newDebugTraceTestClient(t)
	var out bytes.Buffer

	// Emit a trace task before capture.
	_, task := trace.NewTask(ctx, "debug-trace-test")
	trace.Log(ctx, "phase", "capture")
	task.End()

	// Capture the runtime trace and require the written byte count.
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

// TestDebugCommandsHonorGlobalSocketPath checks that a --socket-path given
// before the debug command reaches the capture's dial, though each debug
// command declares its own flag: an ignored one falls back to the default
// state path and captures a different daemon.
func TestDebugCommandsHonorGlobalSocketPath(t *testing.T) {
	// Restore the dial hook, and record the socket it is asked for.
	oldDial := connectDaemonDial
	t.Cleanup(func() { connectDaemonDial = oldDial })
	errDialed := errors.New("dialed")
	var dialed string
	connectDaemonDial = func(ctx context.Context, sockPath string) (net.Conn, error) {
		dialed = sockPath
		return nil, errDialed
	}

	// Run each capture with the socket on the root and require that it dials it.
	sock := filepath.Join(t.TempDir(), "spacewave.sock")
	for _, name := range []string{"trace", "cpu-profile", "mem-profile", "goroutine-profile"} {
		dialed = ""
		app := &cli.App{
			Flags:    []cli.Flag{socketPathFlag()},
			Commands: []*cli.Command{newDebugCommand(nil, &FormatUpgrade{})},
		}
		out := filepath.Join(t.TempDir(), "out")
		err := app.Run([]string{"spacewave", "--socket-path", sock, "debug", name, "--out", out})
		if !errors.Is(err, errDialed) {
			t.Fatalf("%s: got %v, want the stub dial error", name, err)
		}
		if dialed != sock {
			t.Fatalf("%s: dialed %q, want %q", name, dialed, sock)
		}
	}
}

func TestConnectDebugTraceDaemonUsesSocketPathWithoutAutostart(t *testing.T) {
	// Restore the daemon connection hooks after the test.
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	// Pipe a connection and close it when the test ends.
	connA, connB := net.Pipe()
	t.Cleanup(func() {
		connA.Close()
		connB.Close()
	})

	// Stub dial, client build, and start so only dial is used.
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

	// Connect through the socket path and require that dial used it.
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
	// Build a trace client and output buffer.
	ctx := context.Background()
	traceClient := newDebugTraceTestClient(t)
	var out bytes.Buffer

	// Capture a CPU profile and require the written byte count.
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
	// Build a trace client and output buffer.
	ctx := context.Background()
	traceClient := newDebugTraceTestClient(t)
	var out bytes.Buffer

	// Capture an allocs profile and require the written byte count.
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

func TestCaptureDaemonGoroutineProfileWritesProfile(t *testing.T) {
	// Build a trace client and a goroutine that the profile must list.
	ctx := context.Background()
	traceClient := newDebugTraceTestClient(t)
	var out bytes.Buffer
	started, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { close(release) })
	go debugGoroutineProfileMarker(started, release)
	<-started

	// Capture the goroutine profile the way the CLI does.
	byteCount, err := trace_capture.CaptureGoroutineProfile(ctx, traceClient, &out, trace_capture.GoroutineProfileArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if byteCount == 0 || int64(out.Len()) != byteCount {
		t.Fatalf("expected %d profile bytes, got %d", byteCount, out.Len())
	}

	// Parse the bytes with the pprof reader and find the blocked goroutine.
	prof, err := profile.Parse(&out)
	if err != nil {
		t.Fatalf("parse goroutine profile: %v", err)
	}
	if len(prof.Sample) == 0 || len(prof.SampleType) == 0 || prof.SampleType[0].Type != "goroutine" {
		t.Fatalf("unexpected goroutine profile: %d samples, types %v", len(prof.Sample), prof.SampleType)
	}
	var found bool
	for _, sample := range prof.Sample {
		for _, loc := range sample.Location {
			for _, line := range loc.Line {
				if strings.HasSuffix(line.Function.Name, "debugGoroutineProfileMarker") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("goroutine profile does not list the marker goroutine")
	}
}

// debugGoroutineProfileMarker closes started once running, then blocks until
// release closes so the goroutine profile has a known stack to find.
func debugGoroutineProfileMarker(started chan<- struct{}, release <-chan struct{}) {
	close(started)
	<-release
}

func TestDefaultDebugGoroutineProfileOutputPath(t *testing.T) {
	got := defaultDebugGoroutineProfileOutputPath(time.Date(2026, 5, 11, 18, 20, 30, 0, time.UTC))
	want := ".tmp/spacewave-daemon-20260511-182030-goroutine.pprof"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func newDebugTraceTestClient(t *testing.T) s4wave_trace.SRPCTraceServiceClient {
	// Mark the helper.
	t.Helper()

	// Register the trace service on a mux.
	mux := srpc.NewMux()
	if err := s4wave_trace.SRPCRegisterTraceService(mux, trace_service.NewService()); err != nil {
		t.Fatal(err)
	}

	// Pipe a server and client connection.
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

	// Accept the server connection and return the trace client.
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.AcceptMuxedConn(serverCtx, serverMux)
	}()
	t.Cleanup(func() {
		// Close the mux and require a clean shutdown.
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
