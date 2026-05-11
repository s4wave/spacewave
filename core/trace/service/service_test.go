package trace_service

import (
	"bytes"
	"context"
	"errors"
	"io"
	runtime_trace "runtime/trace"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	s4wave_trace "github.com/s4wave/spacewave/sdk/trace"
)

type blockingTraceStream struct {
	ctx       context.Context
	firstSent chan struct{}
	release   chan struct{}
	firstCopy []byte
	firstMsg  []byte
}

func newBlockingTraceStream(ctx context.Context) *blockingTraceStream {
	return &blockingTraceStream{
		ctx:       ctx,
		firstSent: make(chan struct{}),
		release:   make(chan struct{}),
	}
}

func (s *blockingTraceStream) Context() context.Context { return s.ctx }

func (s *blockingTraceStream) MsgSend(msg srpc.Message) error {
	resp, ok := msg.(*s4wave_trace.StopTraceResponse)
	if !ok {
		return errors.New("unexpected trace response message")
	}
	return s.Send(resp)
}

func (s *blockingTraceStream) MsgRecv(srpc.Message) error { return io.EOF }

func (s *blockingTraceStream) CloseSend() error { return nil }

func (s *blockingTraceStream) Close() error { return nil }

func (s *blockingTraceStream) Send(resp *s4wave_trace.StopTraceResponse) error {
	if s.firstMsg == nil {
		s.firstMsg = resp.GetData()
		s.firstCopy = bytes.Clone(s.firstMsg)
		close(s.firstSent)
		<-s.release
	}
	return nil
}

func (s *blockingTraceStream) SendAndClose(resp *s4wave_trace.StopTraceResponse) error {
	if resp != nil {
		return s.Send(resp)
	}
	return nil
}

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

func TestTraceServiceStopTraceOwnsStreamedBytes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	service := NewService()

	_, err := service.StartTrace(ctx, &s4wave_trace.StartTraceRequest{Label: "first"})
	if err != nil {
		t.Fatal(err)
	}
	traceCtx, task := runtime_trace.NewTask(ctx, "first-work")
	for range 256 {
		runtime_trace.Log(traceCtx, "phase", "first")
	}
	task.End()

	strm := newBlockingTraceStream(ctx)
	defer func() {
		select {
		case <-strm.release:
		default:
			close(strm.release)
		}
	}()
	errCh := make(chan error, 1)
	go func() {
		errCh <- service.StopTrace(&s4wave_trace.StopTraceRequest{}, strm)
	}()

	select {
	case <-strm.firstSent:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	_, err = service.StartTrace(ctx, &s4wave_trace.StartTraceRequest{Label: "second"})
	if err != nil {
		t.Fatal(err)
	}
	secondCtx, secondTask := runtime_trace.NewTask(ctx, "second-work")
	for range 256 {
		runtime_trace.Log(secondCtx, "phase", "second")
	}
	secondTask.End()

	if !bytes.Equal(strm.firstMsg, strm.firstCopy) {
		t.Fatal("first streamed trace chunk mutated after starting replacement trace")
	}

	close(strm.release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	drain := newTestTraceClient(t, service)
	stopStrm, err := drain.StopTrace(ctx, &s4wave_trace.StopTraceRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for {
		_, err := stopStrm.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}
