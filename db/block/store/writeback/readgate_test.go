package block_store_writeback

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestReadGateFollowsLoad raises the bound by fast reads up to the policy's
// highest, halves it once per slow spell, keeps it at least one, admits no
// more reads than the bound, and keeps a batch's reads within the bound.
func TestReadGateFollowsLoad(t *testing.T) {
	// Fast reads raise the bound to the highest.
	ctx := context.Background()
	g := newReadGate()
	g.setPolicy(4, time.Millisecond)
	acquire := func() {
		if err := g.acquire(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for range 100 {
		acquire()
		g.release(time.Now(), 100*time.Microsecond)
	}
	if limit := g.getLimit(); limit != 4 {
		t.Fatalf("bound after fast reads is %d, want 4", limit)
	}

	// Slow reads that started before a cut cut no further.
	started := time.Now().Add(-time.Millisecond)
	for range 3 {
		acquire()
		g.release(started, 10*time.Millisecond)
	}
	if limit := g.getLimit(); limit != 2 {
		t.Fatalf("bound after one slow spell is %d, want 2", limit)
	}

	// Later slow spells halve down to one and no lower.
	for range 4 {
		time.Sleep(time.Millisecond)
		acquire()
		g.release(time.Now(), 10*time.Millisecond)
	}
	if limit := g.getLimit(); limit != 1 {
		t.Fatalf("bound after slow spells is %d, want 1", limit)
	}

	// The lowered bound admits one read at a time.
	if !g.slots.TryAcquire(1) {
		t.Fatal("a bound of one admitted no read")
	}
	if g.slots.TryAcquire(1) {
		t.Fatal("a bound of one admitted a second read")
	}
	g.slots.Release(1)

	// A batch of reads never runs more at once than the bound allows.
	var open, most atomic.Int32
	var ran atomic.Int32
	err := g.readEach(ctx, 200, func(context.Context, int) error {
		n := open.Add(1)
		defer open.Add(-1)
		for m := most.Load(); n > m && !most.CompareAndSwap(m, n); m = most.Load() {
		}
		ran.Add(1)
		return nil
	})
	if err != nil || ran.Load() != 200 || most.Load() > 4 {
		t.Fatalf("batch ran %d reads, at most %d at once: %v", ran.Load(), most.Load(), err)
	}
}
