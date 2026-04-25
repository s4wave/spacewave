package trace_service

import (
	"context"
	"io"
	runtime_trace "runtime/trace"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	s4wave_trace "github.com/s4wave/spacewave/sdk/trace"
)

func newTestTraceClient(t *testing.T, impl s4wave_trace.SRPCTraceServiceServer) s4wave_trace.SRPCTraceServiceClient {
	t.Helper()

	mux := srpc.NewMux()
	if err := s4wave_trace.SRPCRegisterTraceService(mux, impl); err != nil {
		t.Fatal(err)
	}

	server := srpc.NewServer(mux)
	client := srpc.NewClient(srpc.NewServerPipe(server))
	return s4wave_trace.NewSRPCTraceServiceClient(client)
}

func TestTraceServiceSinglePlugin(t *testing.T) {
	ctx := context.Background()
	client := newTestTraceClient(t, NewService())

	_, err := client.StartTrace(ctx, &s4wave_trace.StartTraceRequest{Label: "single-plugin"})
	if err != nil {
		t.Fatal(err)
	}

	traceCtx, task := runtime_trace.NewTask(ctx, "single-plugin-work")
	runtime_trace.Log(traceCtx, "phase", "single-plugin")
	runtime_trace.StartRegion(traceCtx, "single-plugin-region").End()
	task.End()

	stopStrm, err := client.StopTrace(ctx, &s4wave_trace.StopTraceRequest{})
	if err != nil {
		t.Fatal(err)
	}

	var traceData []byte
	for {
		msg, err := stopStrm.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		traceData = append(traceData, msg.GetData()...)
	}

	if len(traceData) == 0 {
		t.Fatal("expected non-empty trace data")
	}
}

func TestTraceServiceReplaceActive(t *testing.T) {
	ctx := context.Background()
	client := newTestTraceClient(t, NewService())

	// Start first trace.
	_, err := client.StartTrace(ctx, &s4wave_trace.StartTraceRequest{Label: "first"})
	if err != nil {
		t.Fatal(err)
	}

	// Replace with a second trace without stopping.
	_, err = client.StartTrace(ctx, &s4wave_trace.StartTraceRequest{Label: "second"})
	if err != nil {
		t.Fatal(err)
	}

	// Emit work only under the second trace.
	traceCtx, task := runtime_trace.NewTask(ctx, "replace-work")
	runtime_trace.Log(traceCtx, "phase", "replace")
	task.End()

	// Stop and collect.
	stopStrm, err := client.StopTrace(ctx, &s4wave_trace.StopTraceRequest{})
	if err != nil {
		t.Fatal(err)
	}

	var traceData []byte
	for {
		msg, err := stopStrm.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		traceData = append(traceData, msg.GetData()...)
	}

	if len(traceData) == 0 {
		t.Fatal("expected non-empty trace data from replaced trace")
	}
}
