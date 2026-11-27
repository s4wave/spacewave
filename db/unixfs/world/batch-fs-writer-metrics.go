package unixfs_world

import "context"

// BatchFSWriterMetric records one memory-relevant batch upload stage.
type BatchFSWriterMetric struct {
	// Stage records the point in the batch lifecycle when the metric was
	// taken.
	Stage string
	// Bytes is the size of the file ingested, on the ingest stages.
	Bytes int64
	// PendingDirs is how many directories the batch touches.
	PendingDirs int
	// PendingEntries is how many entries the batch holds.
	PendingEntries int
	// Released is set once the writer was released.
	Released bool
	// Committed is set once the batch was published or found empty.
	Committed bool
	// Attempt counts the merges of one Commit, from 1, on commit-merged.
	Attempt int
}

// BatchFSWriterMetricsRecorder consumes batch writer metrics.
type BatchFSWriterMetricsRecorder interface {
	// RecordBatchFSWriterMetric records one metric on the writer's
	// goroutine, inside the commit's merge for commit-merged.
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
