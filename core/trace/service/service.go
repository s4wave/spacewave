package trace_service

import (
	"bytes"
	"context"
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
	mu          sync.Mutex
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
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.active {
		runtime_trace.Stop()
		s.active = false
	}

	s.buf.Reset()
	if err := runtime_trace.Start(&s.buf); err != nil {
		return nil, err
	}

	s.active = true
	return &s4wave_trace.StartTraceResponse{}, nil
}

// StopTrace stops runtime trace capture and streams the captured bytes.
func (s *Service) StopTrace(_ *s4wave_trace.StopTraceRequest, strm s4wave_trace.SRPCTraceService_StopTraceStream) error {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return errors.New("trace not active")
	}

	runtime_trace.Stop()
	data := bytes.Clone(s.buf.Bytes())
	s.buf.Reset()
	s.active = false
	s.mu.Unlock()

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
	duration := time.Duration(req.GetDurationMillis()) * time.Millisecond
	if duration <= 0 {
		return errors.New("duration must be greater than zero")
	}

	var buf bytes.Buffer
	s.mu.Lock()
	if s.profileBusy {
		s.mu.Unlock()
		return errors.New("cpu profile already active")
	}
	if err := pprof.StartCPUProfile(&buf); err != nil {
		s.mu.Unlock()
		return err
	}
	s.profileBusy = true
	s.mu.Unlock()

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

	s.mu.Lock()
	pprof.StopCPUProfile()
	data := bytes.Clone(buf.Bytes())
	s.profileBusy = false
	s.mu.Unlock()

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

// _ is a type assertion
var _ s4wave_trace.SRPCTraceServiceServer = (*Service)(nil)
