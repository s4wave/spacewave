package block

import (
	"context"
	"sync/atomic"
)

type writeCounterContextKey struct{}

// WriteCounter records aggregate block writes for one logical operation.
type WriteCounter struct {
	// count is the number of encoded blocks a transaction submitted for storage.
	count atomic.Uint64
	// bytes is the number of encoded bytes a transaction submitted for storage.
	bytes atomic.Uint64
}

// WriteCounterSnapshot is a point-in-time copy of a WriteCounter.
type WriteCounterSnapshot struct {
	// BlockWriteCount is the number of encoded blocks submitted for storage.
	BlockWriteCount uint64
	// BlockWriteBytes is the number of encoded bytes submitted for storage.
	BlockWriteBytes uint64
}

// WithWriteCounter returns a context that records aggregate block writes.
func WithWriteCounter(ctx context.Context) (context.Context, *WriteCounter) {
	if ctx == nil {
		ctx = context.Background()
	}
	counter := &WriteCounter{}
	return context.WithValue(ctx, writeCounterContextKey{}, counter), counter
}

// Snapshot returns the current counter values.
func (c *WriteCounter) Snapshot() WriteCounterSnapshot {
	if c == nil {
		return WriteCounterSnapshot{}
	}
	return WriteCounterSnapshot{
		BlockWriteCount: c.count.Load(),
		BlockWriteBytes: c.bytes.Load(),
	}
}

// RecordWrite records one encoded block submitted for storage on the
// counter attached to ctx, if any.
func RecordWrite(ctx context.Context, bytes int) {
	// Skip when no counter is attached.
	counter, _ := ctx.Value(writeCounterContextKey{}).(*WriteCounter)
	if counter == nil {
		return
	}

	// Add the block and its bytes.
	counter.count.Add(1)
	counter.bytes.Add(nonNegativeReadBytes(bytes))
}
