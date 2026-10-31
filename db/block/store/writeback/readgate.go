package block_store_writeback

import (
	"context"
	"sync"
	"time"
)

// defaultMaxReads bounds the local reads an upload keeps in flight when the
// policy sets no bound.
const defaultMaxReads = 32

// readFloorWindow is how long the fastest read seen stays the floor. A floor
// older than two windows gives way to the fastest read of the last window,
// so a disk that slowed for good stops counting as loaded.
const readFloorWindow = 10 * time.Second

// readSlowFactor is how many times the floor a read may take before the gate
// counts the disk as loaded.
const readSlowFactor = 3

// defaultReadSlow is the least read time the gate counts as slow when the
// policy sets none, so a floor of cached reads does not count every disk read
// as slow.
const defaultReadSlow = 5 * time.Millisecond

// readGate bounds the local reads the upload keeps in flight and moves the
// bound with the disk's load: a read slower than readSlowFactor times the
// fastest recent read halves it, and a full bound of fast reads raises it by
// one. The store's own reads share the disk, so the upload yields to them
// while they queue and takes the disk while it is idle.
type readGate struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// max is the highest bound, from the policy.
	max int
	// slowMin is the least read time counted as slow, from the policy.
	slowMin time.Duration
	// limit is the current bound, from one to max.
	limit int
	// inflight is the number of reads holding the gate.
	inflight int
	// fast counts the fast reads since the bound last changed.
	fast int
	// cut is when the bound was last halved. Reads that started before it
	// measured the load the cut answered, so they cut no further.
	cut time.Time
	// floor is the fastest read of the current and last window, zero before
	// the first.
	floor time.Duration
	// windowFloor is the fastest read of the current window.
	windowFloor time.Duration
	// windowStart is when the current window began.
	windowStart time.Time
	// wake closes when a read releases the gate.
	wake chan struct{}
}

// newReadGate constructs a gate starting at a bound of one.
func newReadGate() *readGate {
	return &readGate{max: defaultMaxReads, slowMin: defaultReadSlow, limit: 1, wake: make(chan struct{})}
}

// setPolicy sets the highest bound and the least slow read time, the
// defaults when zero.
func (g *readGate) setPolicy(maxReads int, slowMin time.Duration) {
	// Default the zero values.
	if maxReads <= 0 {
		maxReads = defaultMaxReads
	}
	if slowMin <= 0 {
		slowMin = defaultReadSlow
	}

	// Lower the bound into the new range.
	g.mtx.Lock()
	defer g.mtx.Unlock()
	g.max, g.slowMin = maxReads, slowMin
	g.limit = min(g.limit, g.max)
}

// read runs fn once the gate admits it and adjusts the bound by its time.
func (g *readGate) read(ctx context.Context, fn func() error) error {
	// Wait for a free slot.
	if err := g.acquire(ctx); err != nil {
		return err
	}

	// Time the read and release the slot with its time.
	started := time.Now()
	err := fn()
	g.release(started, time.Since(started))
	return err
}

// acquire waits until fewer reads than the bound hold the gate.
func (g *readGate) acquire(ctx context.Context) error {
	for {
		// Take a slot when one is free.
		g.mtx.Lock()
		if g.inflight < g.limit {
			g.inflight++
			g.mtx.Unlock()
			return nil
		}
		wake := g.wake
		g.mtx.Unlock()

		// Wait for a release.
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-wake:
		}
	}
}

// release frees a slot and moves the bound by the read's time.
func (g *readGate) release(started time.Time, took time.Duration) {
	// Free the slot and fold the read into the floor.
	g.mtx.Lock()
	defer g.mtx.Unlock()
	g.inflight--
	g.observe(started.Add(took), took)

	// Halve the bound on a slow read, or raise it after a full bound of fast
	// ones.
	slow := max(g.floor*readSlowFactor, g.slowMin)
	switch {
	case took > slow && started.After(g.cut):
		g.limit = max(g.limit/2, 1)
		g.cut, g.fast = time.Now(), 0
	case took <= slow:
		g.fast++
		if g.fast >= g.limit && g.limit < g.max {
			g.limit++
			g.fast = 0
		}
	}
	g.wakeLocked()
}

// observe folds a read time into the windowed floor. The caller holds mtx.
func (g *readGate) observe(now time.Time, took time.Duration) {
	// Start a new window when the current one ended, keeping the last
	// window's fastest read as the floor.
	if now.Sub(g.windowStart) >= readFloorWindow {
		if g.windowFloor != 0 {
			g.floor = g.windowFloor
		}
		g.windowStart, g.windowFloor = now, 0
	}

	// Lower both floors to a faster read.
	if g.windowFloor == 0 || took < g.windowFloor {
		g.windowFloor = took
	}
	if g.floor == 0 || took < g.floor {
		g.floor = took
	}
}

// wakeLocked wakes the waiters to retry. The caller holds mtx.
func (g *readGate) wakeLocked() {
	close(g.wake)
	g.wake = make(chan struct{})
}

// getLimit returns the current bound.
func (g *readGate) getLimit() int {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	return g.limit
}
