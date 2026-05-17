package trace_service

import (
	"bytes"
	"context"
	"errors"
	"io"
	runtime_trace "runtime/trace"
	"strings"
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

type testCPUProfileStream struct {
	ctx       context.Context
	sendErr   error
	byteCount int
}

func (s *testCPUProfileStream) Context() context.Context {
	if s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

func (s *testCPUProfileStream) MsgSend(msg srpc.Message) error {
	resp, ok := msg.(*s4wave_trace.CaptureCPUProfileResponse)
	if !ok {
		return errors.New("unexpected CPU profile response message")
	}
	return s.Send(resp)
}

func (s *testCPUProfileStream) MsgRecv(srpc.Message) error { return io.EOF }

func (s *testCPUProfileStream) CloseSend() error { return nil }

func (s *testCPUProfileStream) Close() error { return nil }

func (s *testCPUProfileStream) Send(resp *s4wave_trace.CaptureCPUProfileResponse) error {
	if s.sendErr != nil {
		return s.sendErr
	}
	s.byteCount += len(resp.GetData())
	return nil
}

func (s *testCPUProfileStream) SendAndClose(resp *s4wave_trace.CaptureCPUProfileResponse) error {
	if resp != nil {
		return s.Send(resp)
	}
	return nil
}

type testMemoryProfileStream struct {
	ctx     context.Context
	sendErr error
}

func (s *testMemoryProfileStream) Context() context.Context {
	if s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

func (s *testMemoryProfileStream) MsgSend(msg srpc.Message) error {
	resp, ok := msg.(*s4wave_trace.CaptureMemoryProfileResponse)
	if !ok {
		return errors.New("unexpected memory profile response message")
	}
	return s.Send(resp)
}

func (s *testMemoryProfileStream) MsgRecv(srpc.Message) error { return io.EOF }

func (s *testMemoryProfileStream) CloseSend() error { return nil }

func (s *testMemoryProfileStream) Close() error { return nil }

func (s *testMemoryProfileStream) Send(resp *s4wave_trace.CaptureMemoryProfileResponse) error {
	if s.sendErr != nil {
		return s.sendErr
	}
	return nil
}

func (s *testMemoryProfileStream) SendAndClose(resp *s4wave_trace.CaptureMemoryProfileResponse) error {
	if resp != nil {
		return s.Send(resp)
	}
	return nil
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

func TestTraceServiceCaptureCPUProfile(t *testing.T) {
	ctx := context.Background()
	client := newTestTraceClient(t, NewService())

	strm, err := client.CaptureCPUProfile(ctx, &s4wave_trace.CaptureCPUProfileRequest{
		DurationMillis: 100,
		Label:          "cpu-profile-test",
	})
	if err != nil {
		t.Fatal(err)
	}

	var profileData []byte
	for {
		msg, err := strm.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		profileData = append(profileData, msg.GetData()...)
	}

	if len(profileData) == 0 {
		t.Fatal("expected non-empty CPU profile data")
	}
}

func TestTraceServiceRejectsBusyCPUProfile(t *testing.T) {
	service := NewService()
	service.mu.Lock()
	service.profileBusy = true
	service.mu.Unlock()

	err := service.CaptureCPUProfile(&s4wave_trace.CaptureCPUProfileRequest{DurationMillis: 1}, &testCPUProfileStream{})
	if err == nil || !strings.Contains(err.Error(), "already active") {
		t.Fatalf("expected busy CPU profile error, got %v", err)
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	if !service.profileBusy {
		t.Fatal("busy rejection cleared the active profile owner")
	}
	service.profileBusy = false
}

func TestTraceServiceCPUProfileCancelClearsBusy(t *testing.T) {
	service := NewService()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := service.CaptureCPUProfile(&s4wave_trace.CaptureCPUProfileRequest{DurationMillis: 1000}, &testCPUProfileStream{ctx: ctx})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}

	service.mu.Lock()
	busy := service.profileBusy
	service.mu.Unlock()
	if busy {
		t.Fatal("canceled CPU profile left profileBusy set")
	}

	strm := &testCPUProfileStream{}
	if err := service.CaptureCPUProfile(&s4wave_trace.CaptureCPUProfileRequest{DurationMillis: 1}, strm); err != nil {
		t.Fatal(err)
	}
	if strm.byteCount == 0 {
		t.Fatal("expected follow-up CPU profile bytes after cancellation cleanup")
	}
}

func TestTraceServiceCaptureMemoryProfile(t *testing.T) {
	ctx := context.Background()
	client := newTestTraceClient(t, NewService())

	strm, err := client.CaptureMemoryProfile(ctx, &s4wave_trace.CaptureMemoryProfileRequest{
		Profile: "allocs",
	})
	if err != nil {
		t.Fatal(err)
	}

	var profileData []byte
	for {
		msg, err := strm.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		profileData = append(profileData, msg.GetData()...)
	}

	if len(profileData) == 0 {
		t.Fatal("expected non-empty memory profile data")
	}
}

func TestTraceServiceReturnsStreamingErrors(t *testing.T) {
	sendErr := errors.New("send memory profile chunk")
	err := NewService().CaptureMemoryProfile(
		&s4wave_trace.CaptureMemoryProfileRequest{Profile: "allocs"},
		&testMemoryProfileStream{sendErr: sendErr},
	)
	if !errors.Is(err, sendErr) {
		t.Fatalf("expected streaming error, got %v", err)
	}
}
