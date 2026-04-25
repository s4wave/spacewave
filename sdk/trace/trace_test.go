package s4wave_trace

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
)

type testTraceService struct {
	startLabel string
	stopData   [][]byte
	stopCalls  int
}

func (t *testTraceService) StartTrace(_ context.Context, req *StartTraceRequest) (*StartTraceResponse, error) {
	t.startLabel = req.GetLabel()
	return &StartTraceResponse{}, nil
}

func (t *testTraceService) StopTrace(_ *StopTraceRequest, strm SRPCTraceService_StopTraceStream) error {
	t.stopCalls++
	for _, chunk := range t.stopData {
		if err := strm.Send(&StopTraceResponse{Data: chunk}); err != nil {
			return err
		}
	}
	return nil
}

func newTestTraceClient(t *testing.T, impl SRPCTraceServiceServer) SRPCTraceServiceClient {
	t.Helper()

	mux := srpc.NewMux()
	if err := SRPCRegisterTraceService(mux, impl); err != nil {
		t.Fatal(err)
	}
	if !mux.HasServiceMethod(SRPCTraceServiceServiceID, "StartTrace") {
		t.Fatal("expected StartTrace to be registered")
	}
	if !mux.HasServiceMethod(SRPCTraceServiceServiceID, "StopTrace") {
		t.Fatal("expected StopTrace to be registered")
	}

	server := srpc.NewServer(mux)
	client := srpc.NewClient(srpc.NewServerPipe(server))
	return NewSRPCTraceServiceClient(client)
}

func TestTraceServiceContract(t *testing.T) {
	ctx := context.Background()
	impl := &testTraceService{
		stopData: [][]byte{
			[]byte("trace-"),
			[]byte("bytes"),
		},
	}
	client := newTestTraceClient(t, impl)

	_, err := client.StartTrace(ctx, &StartTraceRequest{Label: "phase1-seed"})
	if err != nil {
		t.Fatal(err)
	}
	if impl.startLabel != "phase1-seed" {
		t.Fatalf("expected start label %q, got %q", "phase1-seed", impl.startLabel)
	}

	stopStrm, err := client.StopTrace(ctx, &StopTraceRequest{})
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

	if impl.stopCalls != 1 {
		t.Fatalf("expected 1 StopTrace call, got %d", impl.stopCalls)
	}
	if !bytes.Equal(traceData, []byte("trace-bytes")) {
		t.Fatalf("expected streamed trace %q, got %q", []byte("trace-bytes"), traceData)
	}
}
