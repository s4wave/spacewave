package resource_unixfs

import "context"

// UploadMetric records one memory-relevant resource upload stage.
type UploadMetric struct {
	Stage string
	Bytes int
}

// UploadMetricsRecorder consumes resource upload metrics.
type UploadMetricsRecorder interface {
	RecordUploadMetric(UploadMetric)
}

type uploadMetricsRecorderKey struct{}

// WithUploadMetricsRecorder attaches a resource upload metrics recorder to ctx.
func WithUploadMetricsRecorder(ctx context.Context, recorder UploadMetricsRecorder) context.Context {
	if recorder == nil {
		return ctx
	}
	return context.WithValue(ctx, uploadMetricsRecorderKey{}, recorder)
}

func recordUploadMetric(ctx context.Context, metric UploadMetric) {
	recorder, _ := ctx.Value(uploadMetricsRecorderKey{}).(UploadMetricsRecorder)
	if recorder != nil {
		recorder.RecordUploadMetric(metric)
	}
}
