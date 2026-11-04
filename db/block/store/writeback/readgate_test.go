package block_store_writeback

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
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

// waitQueued blocks until n reads wait on the gate.
func waitQueued(t *testing.T, g *readGate, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		g.mtx.Lock()
		queued := g.waiters.Len()
		g.mtx.Unlock()
		if queued == n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d reads queued, want %d", queued, n)
		}
		time.Sleep(100 * time.Microsecond)
	}
}

// TestReadGateManyWaiters runs thousands of reads through a small bound: all
// complete, none runs above the bound, and each runs through a bounded set of
// workers.
func TestReadGateManyWaiters(t *testing.T) {
	// Count the reads running at once.
	const bound, total = 4, 5000
	ctx := t.Context()
	newGate := func() *readGate {
		// Keep the bound steady: no read counts as slow.
		g := newReadGate()
		g.setPolicy(bound, time.Hour)
		g.limit = bound
		return g
	}
	var running, peak atomic.Int64
	work := func() {
		now := running.Add(1)
		for {
			old := peak.Load()
			if now <= old || peak.CompareAndSwap(old, now) {
				break
			}
		}
		time.Sleep(10 * time.Microsecond)
		running.Add(-1)
	}

	// Queue one goroutine per read, as the old fan-out did.
	g := newGate()
	var wg sync.WaitGroup
	for range total {
		wg.Go(func() {
			if err := g.read(ctx, func() error { work(); return nil }); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if got := peak.Load(); got > bound {
		t.Fatalf("%d reads ran at once, bound is %d", got, bound)
	}

	// Run the same count through each, which visits every index once.
	g = newGate()
	peak.Store(0)
	visits := make([]int, total)
	err := g.each(ctx, total, func(_ context.Context, i int) error {
		work()
		visits[i]++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, n := range visits {
		if n != 1 {
			t.Fatalf("index %d visited %d times", i, n)
		}
	}
	if got := peak.Load(); got > bound {
		t.Fatalf("each ran %d reads at once, bound is %d", got, bound)
	}

	// The first error stops the batch and comes back.
	boom := errors.New("boom")
	var ran atomic.Int64
	err = g.each(ctx, total, func(_ context.Context, i int) error {
		if ran.Add(1) == 10 {
			return boom
		}
		return nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("each returned %v, want %v", err, boom)
	}
	if got := ran.Load(); got >= total {
		t.Fatalf("each ran all %d reads after an error", got)
	}
	if g.inflight != 0 || g.waiters.Len() != 0 {
		t.Fatalf("gate holds %d slots with %d waiters after each", g.inflight, g.waiters.Len())
	}
}

// TestReadGateCancelKeepsSlots cancels a queued waiter, then a waiter granted
// the freed slot as it cancels, and checks the slot reaches the next waiter
// either way.
func TestReadGateCancelKeepsSlots(t *testing.T) {
	// Hold the only slot so a waiter queues.
	g := newReadGate()
	if err := g.acquire(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- g.acquire(ctx) }()
	waitQueued(t, g, 1)

	// Cancel the queued waiter, which leaves the queue.
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled acquire returned %v", err)
	}
	waitQueued(t, g, 0)

	// A waiter canceled as a release grants it the slot either keeps the slot
	// or hands it on, and the next waiter always gets one.
	for range 200 {
		g := newReadGate()
		if err := g.acquire(t.Context()); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		first := make(chan error, 1)
		second := make(chan error, 1)
		go func() { first <- g.acquire(ctx) }()
		waitQueued(t, g, 1)
		go func() { second <- g.acquire(t.Context()) }()
		waitQueued(t, g, 2)

		// Release the holder's slot and cancel in one step.
		g.mtx.Lock()
		cancel()
		g.inflight--
		g.grantLocked()
		g.mtx.Unlock()

		// Free the first waiter's slot when it kept it.
		if err := <-first; err == nil {
			g.release(time.Now(), time.Microsecond)
		}
		select {
		case err := <-second:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("second waiter never got a slot")
		}
		g.release(time.Now(), time.Microsecond)
		if g.inflight != 0 || g.waiters.Len() != 0 {
			t.Fatalf("gate holds %d slots with %d waiters", g.inflight, g.waiters.Len())
		}
	}
}
