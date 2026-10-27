package block_gc

import (
	"errors"
	"testing"
	"time"
)

// TestCollector_MarkIntervalFromPassEnd checks that a budgeted block pass runs
// only once due and counts the next gap from its end, doubled after a pass
// that found nothing unreached.
func TestCollector_MarkIntervalFromPassEnd(t *testing.T) {
	// Root one block so the pass marks it live and finds nothing unreached.
	env := newTestEnv(t)
	ref := env.putBlock(t, "mark-interval")
	if err := env.gcStore.AddGCRef(env.ctx, NodeGCRoot, BlockIRI(ref)); err != nil {
		t.Fatal(err.Error())
	}
	env.flush(t)

	// A pass is not due right after the interval is set.
	const interval = time.Hour
	env.gc.SetMarkInterval(interval, time.Time{})
	stats, err := env.gc.CollectFor(env.ctx, time.Minute)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stats.BlockPasses != 0 {
		t.Fatalf("expected no block pass before the interval, got %d", stats.BlockPasses)
	}

	// A pass a restart restored as overdue runs at once and schedules the
	// next one twice the interval after it ended, since it found nothing to
	// sweep.
	env.gc.SetMarkInterval(interval, time.Now().Add(-interval))
	stats, err = env.gc.CollectFor(env.ctx, time.Minute)
	if err != nil {
		t.Fatal(err.Error())
	}
	ended := time.Now()
	if stats.BlockPasses != 1 || stats.LiveNodes == 0 || stats.UnreachedBlocks != 0 {
		t.Fatalf("expected one sparse block pass, got %+v", stats)
	}
	if env.gc.markBackoff != 1 {
		t.Fatalf("expected backoff 1, got %d", env.gc.markBackoff)
	}
	if next := env.gc.NextMark(); next.After(ended.Add(2*interval)) || next.Before(ended.Add(2*interval-time.Second)) {
		t.Fatalf("expected the next pass 2h after the pass ended, got %v after", next.Sub(ended))
	}
}

// TestCollector_ScheduleMark checks the back-off after sparse, productive and
// failed passes.
func TestCollector_ScheduleMark(t *testing.T) {
	// Take the yields of the Mac's first two passes on October 18.
	c := &Collector{}
	c.SetMarkInterval(time.Minute, time.Time{})
	sparse := &Stats{LiveNodes: 9_087_280, UnreachedBlocks: 166_086}
	productive := &Stats{LiveNodes: 9_064_682, UnreachedBlocks: 3_381_500}

	// Sparse passes double the gap up to 8x.
	for _, want := range []uint{1, 2, 3, 3} {
		c.scheduleMark(sparse, nil)
		if c.markBackoff != want {
			t.Fatalf("expected backoff %d, got %d", want, c.markBackoff)
		}
	}

	// A failed pass keeps the gap; a productive one resets it.
	c.scheduleMark(productive, errors.New("mark failed"))
	if c.markBackoff != 3 {
		t.Fatalf("expected a failed pass to keep backoff 3, got %d", c.markBackoff)
	}
	c.scheduleMark(productive, nil)
	if c.markBackoff != 0 {
		t.Fatalf("expected a productive pass to reset backoff, got %d", c.markBackoff)
	}
	if gap := time.Until(c.nextMark); gap > time.Minute || gap < time.Minute-time.Second {
		t.Fatalf("expected the base interval after a reset, got %v", gap)
	}

	// A pass that blocks of unknown references stopped backs off, however
	// much it left unreached, as solid's passes did on October 27.
	stopped := &Stats{LiveNodes: 9_064_682, UnreachedBlocks: 3_381_500, UnknownBlocks: 30_727}
	c.scheduleMark(stopped, nil)
	if c.markBackoff != 1 {
		t.Fatalf("expected a stopped pass to back off, got %d", c.markBackoff)
	}
}
