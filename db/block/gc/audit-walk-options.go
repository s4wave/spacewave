package block_gc

import (
	"context"
	"time"
)

// WalkOptions configure one streaming audit walk.
type WalkOptions struct {
	// Run names the walk. A scratch store holding another run's progress is
	// refused; one holding this run's resumes it. Required.
	Run string
	// Delay is how long a hole candidate waits before the walk rereads it.
	// Zero uses DefaultGrace, the collector's own wait for a record's writer.
	// A negative delay rereads at once, which suits a stopped copy that no
	// writer changes.
	Delay time.Duration
	// Now is the walk's clock. Nil uses time.Now.
	Now func() time.Time
	// Wait waits for d or until ctx ends. Nil waits on a timer.
	Wait func(ctx context.Context, d time.Duration) error
}
