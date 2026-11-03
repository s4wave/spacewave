package pace

import "time"

// Stats is a monotonic sample of foreground storage pressure. A counter reset
// begins a fresh window; callers must supply counters from the same database.
type Stats struct {
	// Flushes counts completed foreground flushes.
	Flushes uint64
	// FlushTime sums their elapsed time.
	FlushTime time.Duration
	// BuildWaits counts commits that waited for a checkpoint.
	BuildWaits uint64
}
