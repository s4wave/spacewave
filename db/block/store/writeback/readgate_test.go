package block_store_writeback

import (
	"testing"
	"time"
)

// TestReadGateFollowsLoad raises the bound by fast reads up to the policy's
// highest, halves it once per slow spell, and keeps it at least one.
func TestReadGateFollowsLoad(t *testing.T) {
	// Fast reads raise the bound to the highest.
	g := newReadGate()
	g.setPolicy(4, time.Millisecond)
	fast := func() {
		g.mtx.Lock()
		g.inflight++
		g.mtx.Unlock()
		g.release(time.Now(), 100*time.Microsecond)
	}
	for range 100 {
		fast()
	}
	if limit := g.getLimit(); limit != 4 {
		t.Fatalf("bound after fast reads is %d, want 4", limit)
	}

	// Slow reads that started before a cut cut no further.
	started := time.Now().Add(-time.Millisecond)
	for range 3 {
		g.mtx.Lock()
		g.inflight++
		g.mtx.Unlock()
		g.release(started, 10*time.Millisecond)
	}
	if limit := g.getLimit(); limit != 2 {
		t.Fatalf("bound after one slow spell is %d, want 2", limit)
	}

	// Later slow spells halve down to one and no lower.
	for range 4 {
		time.Sleep(time.Millisecond)
		g.mtx.Lock()
		g.inflight++
		g.mtx.Unlock()
		g.release(time.Now(), 10*time.Millisecond)
	}
	if limit := g.getLimit(); limit != 1 {
		t.Fatalf("bound after slow spells is %d, want 1", limit)
	}
}
