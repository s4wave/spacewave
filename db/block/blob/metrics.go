package blob

import "context"

// Metric records one blob-build memory-relevant decision point.
type Metric struct {
	// Stage names the build decision point.
	Stage string
	// InputBytes is the total input data size.
	InputBytes int64
	// ChunkBytes is the total chunked payload size.
	ChunkBytes int
	// RawHighWaterMark is the raw-blob size limit in effect.
	RawHighWaterMark uint64
	// ChunkIndex is the number of chunks produced.
	ChunkIndex int
	// DirectPut reports whether the blob was stored as raw bytes.
	DirectPut bool
}

// MetricsRecorder consumes blob-build metrics.
type MetricsRecorder interface {
	RecordBlobMetric(Metric)
}

type metricsRecorderKey struct{}

// WithMetricsRecorder attaches a blob metrics recorder to ctx.
func WithMetricsRecorder(ctx context.Context, recorder MetricsRecorder) context.Context {
	if recorder == nil {
		return ctx
	}
	return context.WithValue(ctx, metricsRecorderKey{}, recorder)
}

func recordMetric(ctx context.Context, metric Metric) {
	recorder, _ := ctx.Value(metricsRecorderKey{}).(MetricsRecorder)
	if recorder != nil {
		recorder.RecordBlobMetric(metric)
	}
}
