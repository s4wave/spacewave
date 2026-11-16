package block_store_writeback

import "testing"

// TestEvictBytesWhileCounting checks that a pass while Counting takes the
// high and low marks as it does after, instead of every uploaded byte.
func TestEvictBytesWhileCounting(t *testing.T) {
	policy := Policy{ResidentHigh: 100, ResidentLow: 40}
	for _, c := range []struct {
		name      string
		pending   int64
		evictable int64
		want      int64
	}{
		{"under the high mark", 10, 80, 0},
		{"at the high mark", 20, 80, 0},
		{"past the high mark", 30, 90, 80},
		{"past the high mark on uploaded bytes alone", 0, 150, 110},
		{"bounded by the uploaded bytes", 200, 10, 10},
	} {
		for _, counting := range []bool{false, true} {
			s := Status{Policy: policy, PendingBytes: c.pending, EvictableBytes: c.evictable, Counting: counting}
			if got := s.EvictBytes(); got != c.want {
				t.Errorf("%s, counting=%v: evict bytes = %d, want %d", c.name, counting, got, c.want)
			}
			var latch EvictLatch
			if got := latch.Bytes(s); got != c.want {
				t.Errorf("%s, counting=%v: latch bytes = %d, want %d", c.name, counting, got, c.want)
			}
		}
	}

	// No high mark evicts nothing, Counting or not.
	s := Status{PendingBytes: 10, EvictableBytes: 90, Counting: true}
	if got := s.EvictBytes(); got != 0 {
		t.Fatalf("evict bytes without a high mark = %d, want 0", got)
	}
}
