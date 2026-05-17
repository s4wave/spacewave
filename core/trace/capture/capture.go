package trace_capture

import (
	"context"
	"io"
	"time"

	"github.com/pkg/errors"
	s4wave_trace "github.com/s4wave/spacewave/sdk/trace"
)

// RuntimeTraceArgs configures a runtime trace capture.
type RuntimeTraceArgs struct {
	Duration    time.Duration
	Label       string
	StopTimeout time.Duration
}

// CPUProfileArgs configures a CPU profile capture.
type CPUProfileArgs struct {
	Duration time.Duration
	Label    string
}

// MemoryProfileArgs configures a memory profile capture.
type MemoryProfileArgs struct {
	Profile string
	GC      bool
	Debug   int32
}

// CaptureRuntimeTrace captures a runtime trace from a TraceService client.
func CaptureRuntimeTrace(
	ctx context.Context,
	traceClient s4wave_trace.SRPCTraceServiceClient,
	out io.Writer,
	args RuntimeTraceArgs,
) (int64, error) {
	if traceClient == nil {
		return 0, errors.New("trace client cannot be nil")
	}
	if args.Duration <= 0 {
		return 0, errors.New("duration must be greater than zero")
	}
	if _, err := traceClient.StartTrace(ctx, &s4wave_trace.StartTraceRequest{Label: args.Label}); err != nil {
		return 0, errors.Wrap(err, "start trace")
	}

	timer := time.NewTimer(args.Duration)
	var waitErr error
	select {
	case <-ctx.Done():
		waitErr = ctx.Err()
	case <-timer.C:
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}

	stopCtx := ctx
	var cancel context.CancelFunc
	if waitErr != nil && args.StopTimeout > 0 {
		stopCtx, cancel = context.WithTimeout(context.Background(), args.StopTimeout)
		defer cancel()
	}
	byteCount, err := StopRuntimeTrace(stopCtx, traceClient, out)
	if err != nil {
		return byteCount, errors.Wrap(err, "stop trace")
	}
	return byteCount, waitErr
}

// CaptureCPUProfile captures a CPU profile from a TraceService client.
func CaptureCPUProfile(
	ctx context.Context,
	traceClient s4wave_trace.SRPCTraceServiceClient,
	out io.Writer,
	args CPUProfileArgs,
) (int64, error) {
	if traceClient == nil {
		return 0, errors.New("trace client cannot be nil")
	}
	if args.Duration <= 0 {
		return 0, errors.New("duration must be greater than zero")
	}
	durationMillis := uint32(args.Duration / time.Millisecond)
	if durationMillis == 0 {
		durationMillis = 1
	}
	strm, err := traceClient.CaptureCPUProfile(ctx, &s4wave_trace.CaptureCPUProfileRequest{
		DurationMillis: durationMillis,
		Label:          args.Label,
	})
	if err != nil {
		return 0, errors.Wrap(err, "capture CPU profile")
	}
	defer strm.Close()

	var byteCount int64
	for {
		resp, err := strm.Recv()
		if err == io.EOF {
			return byteCount, nil
		}
		if err != nil {
			return byteCount, err
		}
		n, err := writeChunk(out, resp.GetData())
		byteCount += n
		if err != nil {
			return byteCount, err
		}
	}
}

// CaptureMemoryProfile captures a memory profile from a TraceService client.
func CaptureMemoryProfile(
	ctx context.Context,
	traceClient s4wave_trace.SRPCTraceServiceClient,
	out io.Writer,
	args MemoryProfileArgs,
) (int64, error) {
	if traceClient == nil {
		return 0, errors.New("trace client cannot be nil")
	}
	if args.Debug < 0 {
		return 0, errors.New("debug must be greater than or equal to zero")
	}
	strm, err := traceClient.CaptureMemoryProfile(ctx, &s4wave_trace.CaptureMemoryProfileRequest{
		Profile: args.Profile,
		Gc:      args.GC,
		Debug:   args.Debug,
	})
	if err != nil {
		return 0, errors.Wrap(err, "capture memory profile")
	}
	defer strm.Close()

	var byteCount int64
	for {
		resp, err := strm.Recv()
		if err == io.EOF {
			return byteCount, nil
		}
		if err != nil {
			return byteCount, err
		}
		n, err := writeChunk(out, resp.GetData())
		byteCount += n
		if err != nil {
			return byteCount, err
		}
	}
}

// StopRuntimeTrace stops a runtime trace and writes the streamed bytes.
func StopRuntimeTrace(ctx context.Context, traceClient s4wave_trace.SRPCTraceServiceClient, out io.Writer) (int64, error) {
	if traceClient == nil {
		return 0, errors.New("trace client cannot be nil")
	}
	strm, err := traceClient.StopTrace(ctx, &s4wave_trace.StopTraceRequest{})
	if err != nil {
		return 0, err
	}
	defer strm.Close()

	var byteCount int64
	for {
		resp, err := strm.Recv()
		if err == io.EOF {
			return byteCount, nil
		}
		if err != nil {
			return byteCount, err
		}
		n, err := writeChunk(out, resp.GetData())
		byteCount += n
		if err != nil {
			return byteCount, err
		}
	}
}

func writeChunk(out io.Writer, data []byte) (int64, error) {
	if len(data) == 0 {
		return 0, nil
	}
	n, err := out.Write(data)
	if err != nil {
		return int64(n), err
	}
	if n != len(data) {
		return int64(n), io.ErrShortWrite
	}
	return int64(n), nil
}
