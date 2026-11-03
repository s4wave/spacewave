package pace

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/sirupsen/logrus"
)

// TestEvictPaceBacksOffSlowFlushes checks that eviction waits while flushes
// run past the bound or a commit waits for a checkpoint, runs on once they
// recover, and runs anyway past the back-off bound.
func TestEvictPaceBacksOffSlowFlushes(t *testing.T) {
	// Pace against stats the test advances, with a sleep that records.
	var st Stats
	var slept time.Duration
	p := &Pace{
		le:       logrus.NewEntry(logrus.New()),
		stats:    func() (Stats, bool) { return st, true },
		flushMax: 50 * time.Millisecond,
		sleep: func(_ context.Context, d time.Duration) error {
			slept += d
			return nil
		},
	}
	ctx := t.Context()

	// The first batch has no window to judge and runs.
	if err := p.Admit(ctx, 0); err != nil || slept != 0 {
		t.Fatalf("first admit = %v after %v", err, slept)
	}

	// Fast flushes run the next batch at once.
	st.Flushes, st.FlushTime = 10, 100*time.Millisecond
	if err := p.Admit(ctx, 1<<20); err != nil || slept != 0 {
		t.Fatalf("fast admit = %v after %v", err, slept)
	}

	// Slow flushes wait one step, then a quiet window runs the batch.
	st.Flushes, st.FlushTime = 20, 2100*time.Millisecond
	if err := p.Admit(ctx, 1<<20); err != nil || slept != evictBackoffStep {
		t.Fatalf("slow admit = %v after %v", err, slept)
	}

	// A commit waiting for a checkpoint backs off too.
	slept = 0
	st.BuildWaits = 1
	if err := p.Admit(ctx, 1<<20); err != nil || slept != evictBackoffStep {
		t.Fatalf("build-wait admit = %v after %v", err, slept)
	}

	// Flushes that stay slow hold the batch only up to the bound.
	slept = 0
	p.stats = func() (Stats, bool) {
		st.Flushes++
		st.FlushTime += time.Second
		return st, true
	}
	if err := p.Admit(ctx, 1<<20); err != nil || slept < evictBackoffMax || slept > evictBackoffMax+evictBackoffStepMax {
		t.Fatalf("stuck admit = %v after %v", err, slept)
	}
}

// TestEvictPaceCapsRate checks that the cap spaces batches by the bytes the
// last one dropped, and that a pause holds a pass until it ends.
func TestEvictPaceCapsRate(t *testing.T) {
	// Cap at 1 MiB a second.
	var slept time.Duration
	p := &Pace{
		le:    logrus.NewEntry(logrus.New()),
		stats: func() (Stats, bool) { return Stats{}, false },
		rate:  1 << 20,
		sleep: func(_ context.Context, d time.Duration) error {
			slept += d
			return nil
		},
	}
	ctx := t.Context()

	// A batch of 2 MiB waits about two seconds before the next.
	if err := p.Admit(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if err := p.Admit(ctx, 2<<20); err != nil || slept < 1900*time.Millisecond || slept > 2*time.Second {
		t.Fatalf("capped admit = %v after %v", err, slept)
	}

	// A paused pace holds until the pass ends.
	p.paused = true
	pctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := p.Admit(pctx, 0); err == nil {
		t.Fatal("paused admit ran")
	}
}

// TestPaceCancellation ends real rate and pressure waits when their pass ends.
func TestPaceCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Wait out a rate cap until the owning eviction is cancelled.
		le := logrus.NewEntry(logrus.New())
		rate := New(le, nil, Config{BytesPerSecond: 1})
		if err := rate.Admit(t.Context(), 0); err != nil {
			t.Fatal(err)
		}
		wait, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- rate.Admit(wait, 100) }()
		synctest.Wait()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}

		// A pressure backoff uses the same context-aware timer.
		var st Stats
		pressure := New(le, func() (Stats, bool) { return st, true }, Config{FlushMax: time.Millisecond})
		if err := pressure.Admit(t.Context(), 0); err != nil {
			t.Fatal(err)
		}
		st.Flushes, st.FlushTime = 1, time.Second
		wait, cancel = context.WithCancel(t.Context())
		go func() { done <- pressure.Admit(wait, 0) }()
		synctest.Wait()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
}
