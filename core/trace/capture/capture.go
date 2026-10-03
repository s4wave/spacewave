package trace_capture

import (
	"context"
	"io"
	"math"
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
	// Validate the runtime trace request and start the remote capture.
	if traceClient == nil {
		return 0, errors.New("trace client cannot be nil")
	}
	if args.Duration <= 0 {
		return 0, errors.New("duration must be greater than zero")
	}
	if _, err := traceClient.StartTrace(ctx, &s4wave_trace.StartTraceRequest{Label: args.Label}); err != nil {
		return 0, errors.Wrap(err, "start trace")
	}

	// Wait for the trace duration or caller cancellation and drain the timer.
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

	// Stop the remote trace and collect its bytes even after caller cancellation.
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
	// Validate the CPU profile client and requested duration.
	if traceClient == nil {
		return 0, errors.New("trace client cannot be nil")
	}
	if args.Duration <= 0 {
		return 0, errors.New("duration must be greater than zero")
	}

	// Convert the profile duration to the protocol millisecond field.
	durationMillisValue := args.Duration / time.Millisecond
	if durationMillisValue > math.MaxUint32 {
		return 0, errors.New("duration exceeds trace protocol limit")
	}
	durationMillis := uint32(durationMillisValue) //nolint:gosec // the preceding MaxUint32 check protects the trace request field.
	if durationMillis == 0 {
		durationMillis = 1
	}

	// Start the remote CPU profile stream with its bounded duration.
	strm, err := traceClient.CaptureCPUProfile(ctx, &s4wave_trace.CaptureCPUProfileRequest{
		DurationMillis: durationMillis,
		Label:          args.Label,
	})
	if err != nil {
		return 0, errors.Wrap(err, "capture CPU profile")
	}
	defer strm.Close()

	// Copy CPU profile chunks to the output and count the written bytes.
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
	// Validate the memory profile request and open its remote stream.
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

	// Copy memory profile chunks to the output and count the written bytes.
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
	// Stop the remote runtime trace and acquire its result stream.
	if traceClient == nil {
		return 0, errors.New("trace client cannot be nil")
	}
	strm, err := traceClient.StopTrace(ctx, &s4wave_trace.StopTraceRequest{})
	if err != nil {
		return 0, err
	}
	defer strm.Close()

	// Copy runtime trace chunks to the output and count the written bytes.
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
	// Write the complete profile chunk and report a short output write.
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
