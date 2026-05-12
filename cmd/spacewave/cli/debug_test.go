//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"runtime/trace"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
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

	byteCount, err := captureDaemonRuntimeTrace(
		ctx,
		traceClient,
		&out,
		time.Millisecond,
		"debug-trace-test",
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

func newDebugTraceTestClient(t *testing.T) s4wave_trace.SRPCTraceServiceClient {
	t.Helper()

	mux := srpc.NewMux()
	if err := s4wave_trace.SRPCRegisterTraceService(mux, trace_service.NewService()); err != nil {
		t.Fatal(err)
	}

	server := srpc.NewServer(mux)
	client := srpc.NewClient(srpc.NewServerPipe(server))
	return s4wave_trace.NewSRPCTraceServiceClient(client)
}
