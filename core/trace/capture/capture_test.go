package trace_capture_test

import (
	"bytes"
	"context"
	runtime_trace "runtime/trace"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	trace_capture "github.com/s4wave/spacewave/core/trace/capture"
	trace_service "github.com/s4wave/spacewave/core/trace/service"
	s4wave_trace "github.com/s4wave/spacewave/sdk/trace"
)

func newCaptureTestClient(t *testing.T) s4wave_trace.SRPCTraceServiceClient {
	t.Helper()

	mux := srpc.NewMux()
	if err := s4wave_trace.SRPCRegisterTraceService(mux, trace_service.NewService()); err != nil {
		t.Fatal(err)
	}
	server := srpc.NewServer(mux)
	client := srpc.NewClient(srpc.NewServerPipe(server))
	return s4wave_trace.NewSRPCTraceServiceClient(client)
}

func TestCaptureRuntimeTraceWritesBytes(t *testing.T) {
	ctx := context.Background()
	client := newCaptureTestClient(t)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}
			traceCtx, task := runtime_trace.NewTask(ctx, "capture-helper-work")
			runtime_trace.Log(traceCtx, "phase", "capture-helper")
			task.End()
		}
	}()
	var buf bytes.Buffer
	count, err := trace_capture.CaptureRuntimeTrace(ctx, client, &buf, trace_capture.RuntimeTraceArgs{
		Duration:    10 * time.Millisecond,
		Label:       "capture-helper-trace",
		StopTimeout: time.Second,
	})
	close(done)
	if err != nil {
		t.Fatal(err)
	}
	if count <= 0 || buf.Len() == 0 {
		t.Fatalf("trace bytes: count=%d len=%d", count, buf.Len())
	}
}

func TestCaptureCPUProfileWritesBytes(t *testing.T) {
	ctx := context.Background()
	client := newCaptureTestClient(t)
	var buf bytes.Buffer
	count, err := trace_capture.CaptureCPUProfile(ctx, client, &buf, trace_capture.CPUProfileArgs{
		Duration: 10 * time.Millisecond,
		Label:    "capture-helper-cpu",
	})
	if err != nil {
		t.Fatal(err)
	}
	if count <= 0 || buf.Len() == 0 {
		t.Fatalf("CPU profile bytes: count=%d len=%d", count, buf.Len())
	}
}

func TestCaptureMemoryProfileWritesBytes(t *testing.T) {
	ctx := context.Background()
	client := newCaptureTestClient(t)
	var buf bytes.Buffer
	count, err := trace_capture.CaptureMemoryProfile(ctx, client, &buf, trace_capture.MemoryProfileArgs{
		Profile: "allocs",
	})
	if err != nil {
		t.Fatal(err)
	}
	if count <= 0 || buf.Len() == 0 {
		t.Fatalf("memory profile bytes: count=%d len=%d", count, buf.Len())
	}
}
