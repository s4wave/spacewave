package block_store_writeback

// EvictLatch carries an eviction loop from the high mark down to the low mark
// across passes: once the resident bytes cross the high mark, Bytes keeps
// returning the bytes down to the low mark after a pass cut short leaves them
// between the marks, until they reach the low mark, no uploaded byte is left to
// evict, or either mark changes. Without it a pass that ends early, such as on
// a busy database, starts again only once the resident bytes cross the high
// mark again. The latch decides only when a pass starts. Each pass still
// commits in the same short batches. The zero value is released.
type EvictLatch struct {
	// held is set while a pass started at the high mark has yet to reach
	// the low mark.
	held bool
	// high and low are the marks the latch was set under.
	high, low int64
}

// Bytes returns how many uploaded bytes the next pass drops: EvictBytes when
// positive, which sets the latch, or else, while the latch holds under the same
// marks, the bytes from the resident bytes down to the low mark. Otherwise it
// releases the latch and returns zero.
func (l *EvictLatch) Bytes(s Status) int64 {
	// Set the latch when the store is past the high mark.
	if want := s.EvictBytes(); want > 0 {
		l.held, l.high, l.low = true, s.Policy.ResidentHigh, s.Policy.ResidentLow
		return want
	}

	// Keep evicting toward the low mark while the latch holds under the same
	// marks and there is something to drop.
	var want int64
	if l.held && s.Policy.ResidentHigh == l.high && s.Policy.ResidentLow == l.low {
		want = min(s.ResidentBytes()-s.Policy.ResidentLow, s.EvictableBytes)
	}
	if want <= 0 {
		l.held, want = false, 0
	}
	return want
}
