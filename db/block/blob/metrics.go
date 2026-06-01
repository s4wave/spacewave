package blob

import "context"

// Metric records one blob-build memory-relevant decision point.
type Metric struct {
	Stage            string
	InputBytes       int64
	ChunkBytes       int
	RawHighWaterMark uint64
	ChunkIndex       int
	DirectPut        bool
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
