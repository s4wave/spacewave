package block_store_writeback

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestReadGateFollowsLoad raises the bound by fast reads up to the policy's
// highest, halves it once per slow spell, and keeps it at least one. At the
// bound it hands each freed slot to the oldest waiting read, skips a read
// canceled while it waits, and keeps a batch's reads within the bound.
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

	// Queue three reads behind a held slot and cancel the second.
	ctx := context.Background()
	if err := g.acquire(ctx); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	got := make(chan int, 3)
	for i, ctx := range []context.Context{ctx, canceled, ctx} {
		go func() {
			if g.acquire(ctx) == nil {
				got <- i
			}
		}()
		for waiting := 0; waiting != i+1; {
			g.mtx.Lock()
			waiting = len(g.waiters)
			g.mtx.Unlock()
		}
	}
	cancel()
	for waiting := 3; waiting != 2; {
		g.mtx.Lock()
		waiting = len(g.waiters)
		g.mtx.Unlock()
	}

	// Each release admits the next live waiter alone.
	for _, want := range []int{0, 2} {
		g.release(time.Now(), 10*time.Millisecond)
		if i := <-got; i != want {
			t.Fatalf("release admitted read %d, want %d", i, want)
		}
		g.mtx.Lock()
		inflight := g.inflight
		g.mtx.Unlock()
		if inflight != 1 {
			t.Fatalf("%d reads hold a bound of one", inflight)
		}
	}
	g.release(time.Now(), 10*time.Millisecond)

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
