package trace_service

import (
	"bytes"
	"context"
	"runtime"
	"runtime/pprof"
	runtime_trace "runtime/trace"
	"sync"
	"time"

	"github.com/pkg/errors"
	s4wave_trace "github.com/s4wave/spacewave/sdk/trace"
)

// maxChunkSize is the maximum number of trace bytes per streamed chunk.
const maxChunkSize = 4096

// Service provides process-local runtime diagnostic capture.
type Service struct {
	mtx         sync.Mutex
	buf         bytes.Buffer
	active      bool
	profileBusy bool
}

// NewService constructs a new Service.
func NewService() *Service {
	return &Service{}
}

// StartTrace starts runtime trace capture in the current process.
// If a trace is already active it is stopped and discarded first.
func (s *Service) StartTrace(_ context.Context, _ *s4wave_trace.StartTraceRequest) (*s4wave_trace.StartTraceResponse, error) {
	// Serialize trace replacement against other Service operations.
	s.mtx.Lock()
	defer s.mtx.Unlock()

	// Stop the previous runtime trace before replacing its buffer.
	if s.active {
		runtime_trace.Stop()
		s.active = false
	}

	// Start a runtime trace with an empty Service buffer.
	s.buf.Reset()
	if err := runtime_trace.Start(&s.buf); err != nil {
		return nil, err
	}

	// Publish the active trace state to subsequent Service calls.
	s.active = true
	return &s4wave_trace.StartTraceResponse{}, nil
}

// StopTrace stops runtime trace capture and streams the captured bytes.
func (s *Service) StopTrace(_ *s4wave_trace.StopTraceRequest, strm s4wave_trace.SRPCTraceService_StopTraceStream) error {
	// Require an active Service trace before stopping its capture.
	s.mtx.Lock()
	if !s.active {
		s.mtx.Unlock()
		return errors.New("trace not active")
	}

	// Detach the completed runtime trace bytes before unlocking the Service.
	runtime_trace.Stop()
	data := bytes.Clone(s.buf.Bytes())
	s.buf.Reset()
	s.active = false
	s.mtx.Unlock()

	// Stream the completed trace in bounded response chunks.
	for len(data) > 0 {
		chunk := data
		if len(chunk) > maxChunkSize {
			chunk = chunk[:maxChunkSize]
		}
		data = data[len(chunk):]
		if err := strm.Send(&s4wave_trace.StopTraceResponse{Data: chunk}); err != nil {
			return err
		}
	}
	return nil
}

// CaptureCPUProfile captures a pprof CPU profile in the current process.
func (s *Service) CaptureCPUProfile(
	req *s4wave_trace.CaptureCPUProfileRequest,
	strm s4wave_trace.SRPCTraceService_CaptureCPUProfileStream,
) error {
	// Require a positive capture duration for the CPU profile.
	duration := time.Duration(req.GetDurationMillis()) * time.Millisecond
	if duration <= 0 {
		return errors.New("duration must be greater than zero")
	}

	// Reserve CPU profiling in the Service and start runtime capture.
	var buf bytes.Buffer
	s.mtx.Lock()
	if s.profileBusy {
		s.mtx.Unlock()
		return errors.New("cpu profile already active")
	}
	if err := pprof.StartCPUProfile(&buf); err != nil {
		s.mtx.Unlock()
		return err
	}
	s.profileBusy = true
	s.mtx.Unlock()

	// Wait for the CPU capture duration or stream cancellation.
	timer := time.NewTimer(duration)
	var waitErr error
	select {
	case <-strm.Context().Done():
		waitErr = strm.Context().Err()
	case <-timer.C:
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}

	// Stop CPU capture and release the Service profile reservation.
	s.mtx.Lock()
	pprof.StopCPUProfile()
	data := bytes.Clone(buf.Bytes())
	s.profileBusy = false
	s.mtx.Unlock()

	// Deliver CPU profile chunks unless capture was canceled.
	if waitErr != nil {
		return waitErr
	}
	for len(data) > 0 {
		chunk := data
		if len(chunk) > maxChunkSize {
			chunk = chunk[:maxChunkSize]
		}
		data = data[len(chunk):]
		if err := strm.Send(&s4wave_trace.CaptureCPUProfileResponse{Data: chunk}); err != nil {
			return err
		}
	}
	return nil
}

// CaptureMemoryProfile captures a pprof memory profile in the current process.
func (s *Service) CaptureMemoryProfile(
	req *s4wave_trace.CaptureMemoryProfileRequest,
	strm s4wave_trace.SRPCTraceService_CaptureMemoryProfileStream,
) error {
	// Validate the requested runtime memory profile and capture options.
	profile := req.GetProfile()
	switch profile {
	case "":
		profile = "heap"
	case "heap", "allocs":
	default:
		return errors.Errorf("unsupported memory profile %q", profile)
	}
	debug := req.GetDebug()
	if debug < 0 {
		return errors.New("debug must be greater than or equal to zero")
	}
	if req.GetGc() {
		runtime.GC()
	}
	prof := pprof.Lookup(profile)
	if prof == nil {
		return errors.Errorf("memory profile %q not available", profile)
	}

	// Encode and stream the requested runtime memory profile.
	var buf bytes.Buffer
	if err := prof.WriteTo(&buf, int(debug)); err != nil {
		return errors.Wrap(err, "write memory profile")
	}
	data := bytes.Clone(buf.Bytes())
	for len(data) > 0 {
		chunk := data
		if len(chunk) > maxChunkSize {
			chunk = chunk[:maxChunkSize]
		}
		data = data[len(chunk):]
		if err := strm.Send(&s4wave_trace.CaptureMemoryProfileResponse{Data: chunk}); err != nil {
			return err
		}
	}
	return nil
}

// _ is a type assertion
var _ s4wave_trace.SRPCTraceServiceServer = (*Service)(nil)
