package unixfs_world

import "context"

// BatchFSWriterMetric records one memory-relevant batch upload stage.
type BatchFSWriterMetric struct {
	Stage          string
	Bytes          int64
	PendingDirs    int
	PendingEntries int
	Released       bool
	Committed      bool
}

// BatchFSWriterMetricsRecorder consumes batch writer metrics.
type BatchFSWriterMetricsRecorder interface {
	RecordBatchFSWriterMetric(BatchFSWriterMetric)
}

type batchFSWriterMetricsRecorderKey struct{}

// WithBatchFSWriterMetricsRecorder attaches a batch writer metrics recorder to ctx.
func WithBatchFSWriterMetricsRecorder(
	ctx context.Context,
	recorder BatchFSWriterMetricsRecorder,
) context.Context {
	if recorder == nil {
		return ctx
	}
	return context.WithValue(ctx, batchFSWriterMetricsRecorderKey{}, recorder)
}

func batchFSWriterMetricsRecorder(ctx context.Context) BatchFSWriterMetricsRecorder {
	recorder, _ := ctx.Value(batchFSWriterMetricsRecorderKey{}).(BatchFSWriterMetricsRecorder)
	return recorder
}

func (b *BatchFSWriter) recordBatchFSWriterMetric(ctx context.Context, metric BatchFSWriterMetric) {
	recorder := b.metricsRecorder
	if recorder == nil {
		recorder = batchFSWriterMetricsRecorder(ctx)
		b.metricsRecorder = recorder
	}
	if recorder != nil {
		recorder.RecordBatchFSWriterMetric(metric)
	}
}

func (b *BatchFSWriter) pendingMetricCounts() (int, int) {
	entries := 0
	for _, pd := range b.pending {
		entries += len(pd.entries)
	}
	return len(b.pending), entries
}
