package trace_capture_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	trace_capture "github.com/s4wave/spacewave/core/trace/capture"
	s4wave_trace "github.com/s4wave/spacewave/sdk/trace"
)

type fakeTraceClient struct {
	startReq   *s4wave_trace.StartTraceRequest
	stopData   [][]byte
	cpuReq     *s4wave_trace.CaptureCPUProfileRequest
	cpuData    [][]byte
	memReq     *s4wave_trace.CaptureMemoryProfileRequest
	memoryData [][]byte
}

func TestCaptureRuntimeTraceWritesBytes(t *testing.T) {
	ctx := context.Background()
	client := &fakeTraceClient{
		stopData: [][]byte{[]byte("trace-"), []byte("bytes")},
	}
	var buf bytes.Buffer
	count, err := trace_capture.CaptureRuntimeTrace(ctx, client, &buf, trace_capture.RuntimeTraceArgs{
		Duration:    time.Nanosecond,
		Label:       "capture-helper-trace",
		StopTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.startReq.GetLabel() != "capture-helper-trace" {
		t.Fatalf("trace label = %q", client.startReq.GetLabel())
	}
	if count != int64(len("trace-bytes")) || buf.String() != "trace-bytes" {
		t.Fatalf("trace bytes: count=%d body=%q", count, buf.String())
	}
}

func TestCaptureCPUProfileWritesBytes(t *testing.T) {
	ctx := context.Background()
	client := &fakeTraceClient{
		cpuData: [][]byte{[]byte("cpu-"), []byte("profile")},
	}
	var buf bytes.Buffer
	count, err := trace_capture.CaptureCPUProfile(ctx, client, &buf, trace_capture.CPUProfileArgs{
		Duration: time.Nanosecond,
		Label:    "capture-helper-cpu",
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.cpuReq.GetDurationMillis() != 1 || client.cpuReq.GetLabel() != "capture-helper-cpu" {
		t.Fatalf("cpu request = %+v", client.cpuReq)
	}
	if count != int64(len("cpu-profile")) || buf.String() != "cpu-profile" {
		t.Fatalf("CPU profile bytes: count=%d body=%q", count, buf.String())
	}
}

func TestCaptureMemoryProfileWritesBytes(t *testing.T) {
	ctx := context.Background()
	client := &fakeTraceClient{
		memoryData: [][]byte{[]byte("memory-"), []byte("profile")},
	}
	var buf bytes.Buffer
	count, err := trace_capture.CaptureMemoryProfile(ctx, client, &buf, trace_capture.MemoryProfileArgs{
		Profile: "allocs",
		GC:      true,
		Debug:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.memReq.GetProfile() != "allocs" || !client.memReq.GetGc() || client.memReq.GetDebug() != 1 {
		t.Fatalf("memory request = %+v", client.memReq)
	}
	if count != int64(len("memory-profile")) || buf.String() != "memory-profile" {
		t.Fatalf("memory profile bytes: count=%d body=%q", count, buf.String())
	}
}

func (c *fakeTraceClient) SRPCClient() srpc.Client {
	return nil
}

func (c *fakeTraceClient) StartTrace(_ context.Context, req *s4wave_trace.StartTraceRequest) (*s4wave_trace.StartTraceResponse, error) {
	c.startReq = req.CloneVT()
	return &s4wave_trace.StartTraceResponse{}, nil
}

func (c *fakeTraceClient) StopTrace(ctx context.Context, _ *s4wave_trace.StopTraceRequest) (s4wave_trace.SRPCTraceService_StopTraceClient, error) {
	return &fakeStopTraceStream{fakeTraceStream: fakeTraceStream{ctx: ctx}, data: cloneTraceChunks(c.stopData)}, nil
}

func (c *fakeTraceClient) CaptureCPUProfile(ctx context.Context, req *s4wave_trace.CaptureCPUProfileRequest) (s4wave_trace.SRPCTraceService_CaptureCPUProfileClient, error) {
	c.cpuReq = req.CloneVT()
	return &fakeCPUProfileStream{fakeTraceStream: fakeTraceStream{ctx: ctx}, data: cloneTraceChunks(c.cpuData)}, nil
}

func (c *fakeTraceClient) CaptureMemoryProfile(ctx context.Context, req *s4wave_trace.CaptureMemoryProfileRequest) (s4wave_trace.SRPCTraceService_CaptureMemoryProfileClient, error) {
	c.memReq = req.CloneVT()
	return &fakeMemoryProfileStream{fakeTraceStream: fakeTraceStream{ctx: ctx}, data: cloneTraceChunks(c.memoryData)}, nil
}

type fakeTraceStream struct {
	ctx context.Context
}

func (s fakeTraceStream) Context() context.Context {
	if s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

func (fakeTraceStream) MsgSend(srpc.Message) error { return nil }

func (fakeTraceStream) MsgRecv(srpc.Message) error { return io.EOF }

func (fakeTraceStream) CloseSend() error { return nil }

func (fakeTraceStream) Close() error { return nil }

type fakeStopTraceStream struct {
	fakeTraceStream
	data [][]byte
}

func (s *fakeStopTraceStream) Recv() (*s4wave_trace.StopTraceResponse, error) {
	if len(s.data) == 0 {
		return nil, io.EOF
	}
	data := s.data[0]
	s.data = s.data[1:]
	return &s4wave_trace.StopTraceResponse{Data: data}, nil
}

func (s *fakeStopTraceStream) RecvTo(resp *s4wave_trace.StopTraceResponse) error {
	next, err := s.Recv()
	if err != nil {
		return err
	}
	*resp = *next
	return nil
}

type fakeCPUProfileStream struct {
	fakeTraceStream
	data [][]byte
}

func (s *fakeCPUProfileStream) Recv() (*s4wave_trace.CaptureCPUProfileResponse, error) {
	if len(s.data) == 0 {
		return nil, io.EOF
	}
	data := s.data[0]
	s.data = s.data[1:]
	return &s4wave_trace.CaptureCPUProfileResponse{Data: data}, nil
}

func (s *fakeCPUProfileStream) RecvTo(resp *s4wave_trace.CaptureCPUProfileResponse) error {
	next, err := s.Recv()
	if err != nil {
		return err
	}
	*resp = *next
	return nil
}

type fakeMemoryProfileStream struct {
	fakeTraceStream
	data [][]byte
}

func (s *fakeMemoryProfileStream) Recv() (*s4wave_trace.CaptureMemoryProfileResponse, error) {
	if len(s.data) == 0 {
		return nil, io.EOF
	}
	data := s.data[0]
	s.data = s.data[1:]
	return &s4wave_trace.CaptureMemoryProfileResponse{Data: data}, nil
}

func (s *fakeMemoryProfileStream) RecvTo(resp *s4wave_trace.CaptureMemoryProfileResponse) error {
	next, err := s.Recv()
	if err != nil {
		return err
	}
	*resp = *next
	return nil
}

func cloneTraceChunks(chunks [][]byte) [][]byte {
	out := make([][]byte, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, bytes.Clone(chunk))
	}
	return out
}
