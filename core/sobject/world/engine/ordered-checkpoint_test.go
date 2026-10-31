//go:build !js

package sobject_world_engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
)

// TestOrderedStateWriteWaitsForDurable checks that a state write the
// checkpoint marks ordered returns without flushing the account volume, that
// SOHost.WaitDurable, which every path sending the state off the machine
// calls, returns only after a flush, and that an unmarked state write still
// flushes before it returns.
func TestOrderedStateWriteWaitsForDurable(t *testing.T) {
	// Start a World and let its background writes settle.
	ctx := t.Context()
	w, so := startGatedWorld(t, t.TempDir(), nil)
	w.create(t, "ordered/base")
	host := so.GetSOHost()
	before := quietFlushes(t, w)

	// An ordered state write returns before any flush.
	touch := func(*sobject.SOState) error { return nil }
	if err := host.UpdateSOState(kvtx.WithOrderedCommitContext(ctx), touch); err != nil {
		t.Fatal(err)
	}
	if got := dbFlushes(t, w); got != before {
		t.Fatalf("the ordered state write flushed: %d flushes, want %d", got, before)
	}

	// WaitDurable returns once a flush covers it.
	if err := host.WaitDurable(ctx); err != nil {
		t.Fatal(err)
	}
	durable := dbFlushes(t, w)
	if durable == before {
		t.Fatal("WaitDurable returned before a flush covered the ordered state write")
	}

	// An unmarked state write flushes before it returns.
	if err := host.UpdateSOState(ctx, touch); err != nil {
		t.Fatal(err)
	}
	if got := dbFlushes(t, w); got == durable {
		t.Fatal("the state write returned without a flush")
	}
}

// quietFlushes waits until the account volume has not flushed for longer
// than an ordered commit's flush deadline, and returns its flush count.
func quietFlushes(t *testing.T, w *localWorld) uint64 {
	// Poll the count until it holds still.
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	last, since := dbFlushes(t, w), time.Now()
	for time.Since(since) < 1500*time.Millisecond {
		select {
		case <-ctx.Done():
			t.Fatal("the account volume kept flushing")
		case <-time.After(100 * time.Millisecond):
		}
		if n := dbFlushes(t, w); n != last {
			last, since = n, time.Now()
		}
	}
	return last
}

// dbFlushes returns the account volume's count of flushes to the drive.
func dbFlushes(t *testing.T, w *localWorld) uint64 {
	// Read the volume's stats.
	t.Helper()
	st, err := w.db.Stats(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return st.Flushes
}
