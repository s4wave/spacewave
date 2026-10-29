package s4db

import (
	"sync/atomic"
	"time"
)

// quietMin is the shortest quiet spell after a foreground commit that the
// cleaner waits for.
const quietMin = 10 * time.Millisecond

// quietSpan is how many recent foreground commit times a quiet spell lasts.
const quietSpan = 4

// quiet tracks this handle's foreground commits, so background work runs
// only in the spells between them. It is safe for concurrent use; concurrent
// commits may each miss the other's update of the average, which only
// blurs the estimate.
type quiet struct {
	// active counts the foreground commits running now.
	active atomic.Int32
	// last is the Unix nanosecond time the last foreground commit ended.
	last atomic.Int64
	// avg is the moving average of a foreground commit's duration, in
	// nanoseconds, weighting the newest an eighth.
	avg atomic.Int64
}

// begin records a foreground commit starting and returns the function
// that records its end.
func (q *quiet) begin() func() {
	q.active.Add(1)
	start := time.Now()
	return func() { q.end(start) }
}

// end records a foreground commit that started at start and just ended.
func (q *quiet) end(start time.Time) {
	// Stamp the end and fold the duration into the average.
	now := time.Now()
	defer q.active.Add(-1)
	q.last.Store(now.UnixNano())
	d, avg := int64(now.Sub(start)), q.avg.Load()
	q.avg.Store(avg + (d-avg)/8)
}

// wait returns how long until the quiet spell after the last foreground
// commit ends, or zero or less once it has: quietSpan of the average commit
// time, and at least quietMin. While a commit runs it returns quietMin.
func (q *quiet) wait() time.Duration {
	if q.active.Load() != 0 {
		return quietMin
	}
	span := max(quietMin, quietSpan*time.Duration(q.avg.Load()))
	return time.Until(time.Unix(0, q.last.Load()).Add(span))
}
