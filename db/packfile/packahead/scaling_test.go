package packahead

import (
	"testing"
	"time"
)

// TestPackAheadScaling holds the allocations of Get, Append and Remove flat
// between n and 10n local packs, as the path walk's cost sentences require.
// The calls run with no finalize or rotation beside them, and Append averages
// over enough calls that the maintainer's wakes on its broadcasts, which
// scheduling decides, move the mean by under one allocation. The slack covers
// amortized table growth; a cost per pack would add dozens.
func TestPackAheadScaling(t *testing.T) {
	small := measureCalls(t, 4)
	large := measureCalls(t, 40)
	t.Logf("allocations per call at 4 and 40 packs: %v, %v", small, large)
	for call, n := range small {
		if large[call] > n*1.2+2 {
			t.Errorf("%s: %.1f allocations at 4 packs, %.1f at 40", call, n, large[call])
		}
	}
}

// scalingPackBlocks is the pack size of the scaling test: room for the
// measured Appends in one pack, so none rotates.
const scalingPackBlocks = 64

// measureCalls fills a Log with packs finalized packs and returns the
// allocations per call of each entry point.
func measureCalls(t *testing.T, packs int) map[string]float64 {
	ctx := t.Context()
	l, err := Open(ctx, t.TempDir(), Options{MaxPackBlocks: scalingPackBlocks})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	// Fill the packs and wait for them to finalize.
	next := packs * 1000
	fresh := func() Block {
		next++
		return testBlock(t, next, 3300)
	}
	old := fresh()
	for i := range packs {
		batch := make([]Block, 0, scalingPackBlocks)
		if i == 0 {
			batch = append(batch, old)
		}
		for len(batch) < scalingPackBlocks {
			batch = append(batch, fresh())
		}
		if err := l.Append(ctx, batch); err != nil {
			t.Fatal(err)
		}
	}
	waitFinalized(t, l, packs-1)

	// Rotate the full open pack, then wait for the maintainer to go idle, so no
	// finalize or spare runs beside the measured calls: testing.AllocsPerRun
	// counts every goroutine's allocations.
	if err := l.Append(ctx, []Block{fresh()}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, l)

	// Measure each entry point on the filled Log. Append's 51 calls, warmup
	// included, fit the open pack's 63 free slots.
	pending := make([]Block, 0, 51)
	for range cap(pending) {
		pending = append(pending, fresh())
	}
	i := 0
	return map[string]float64{
		"Get": testing.AllocsPerRun(50, func() {
			if _, ok, err := l.Get(old.Key); err != nil || !ok {
				t.Fatal("get missed", err)
			}
		}),
		"Append": testing.AllocsPerRun(50, func() {
			if err := l.Append(ctx, pending[i:i+1]); err != nil {
				t.Fatal(err)
			}
			i++
		}),
		"Remove": testing.AllocsPerRun(20, func() {
			l.Remove(pending[i%len(pending)].Key)
			i++
		}),
	}
}

// waitIdle waits until the maintainer has no step left: every rotated pack is
// final and the spare is open.
func waitIdle(t *testing.T, l *Log) {
	deadline := time.After(10 * time.Second)
	for {
		var idle bool
		var waitCh <-chan struct{}
		l.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			step, _ := l.nextStepLocked()
			idle, waitCh = step == nil, getWaitCh()
		})
		if idle {
			return
		}
		select {
		case <-waitCh:
		case <-deadline:
			t.Fatalf("maintainer still busy: %v", l.GetStats().MaintainError)
		}
	}
}
