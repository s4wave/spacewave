package block

import (
	"context"
	"sync/atomic"
)

type readCounterContextKey struct{}

// ReadCounter records aggregate block reads for one logical operation.
type ReadCounter struct {
	// count is the number of block reads that returned without store error.
	count atomic.Uint64
	// bytes is the number of bytes returned by found block reads.
	bytes atomic.Uint64
	// misses is the number of block reads that returned not found without store error.
	misses atomic.Uint64
}

// WithReadCounter returns a context that records aggregate block reads.
func WithReadCounter(ctx context.Context) (context.Context, *ReadCounter) {
	if ctx == nil {
		ctx = context.Background()
	}
	counter := &ReadCounter{}
	return context.WithValue(ctx, readCounterContextKey{}, counter), counter
}

// Snapshot returns the current counter values.
func (c *ReadCounter) Snapshot() ReadCounterSnapshot {
	if c == nil {
		return ReadCounterSnapshot{}
	}
	return ReadCounterSnapshot{
		BlockReadCount:     c.count.Load(),
		BlockReadBytes:     c.bytes.Load(),
		BlockReadMissCount: c.misses.Load(),
	}
}

func recordReadCounter(ctx context.Context, found bool, bytes int) {
	counter, _ := ctx.Value(readCounterContextKey{}).(*ReadCounter)
	if counter == nil {
		return
	}
	counter.count.Add(1)
	if found {
		counter.bytes.Add(uint64(bytes))
		return
	}
	counter.misses.Add(1)
}
