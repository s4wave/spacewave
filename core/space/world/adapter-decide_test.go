//go:build !goscript

package space_world_test

import (
	"context"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/world"
)

// TestAdapterDecidesWhileCheckpointStores checks that the driver keeps
// deciding local writes while it stores a large checkpoint and saves its
// cursor in the background: every write that lands before the checkpoint does
// is decided within a quarter of the checkpoint's time, rather than after the
// store, and the log compares them with writes while no checkpoint runs.
func TestAdapterDecidesWhileCheckpointStores(t *testing.T) {
	// Serve a Space and count the operations above its checkpoint.
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()
	f := newAdapterFixture(t)
	_, engRef := f.start(t)
	defer engRef.Release()
	busEngine := world.NewBusEngine(ctx, f.tb.Bus, f.engineID)
	so := f.space(t)
	above := func() int {
		t.Helper()
		snap, err := so.GetSharedObjectState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		set, err := snap.GetOperationSet(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return set.Len()
	}
	var next int
	write := func(objects int) time.Duration {
		t.Helper()
		start := time.Now()
		if err := world.ExecTransaction(ctx, busEngine, true, func(ctx context.Context, ws world.WorldState) error {
			for range objects {
				obj, err := ws.CreateObject(ctx, "decide-"+strconv.Itoa(next), nil)
				world.ReleaseObjectState(obj)
				if err != nil {
					return err
				}
				next++
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return time.Since(start)
	}

	// Write until the driver owes a first checkpoint, then wait for it without
	// writing, so the next counts from it: a write while it stores would stay
	// above it and count toward the next.
	for above() < sobject.MinCheckpointOperations {
		write(1)
	}
	waitCheckpointAbove(ctx, t, so, 0)
	height := checkpointHeight(ctx, t, so)

	// Time single-object writes with no checkpoint running, then grow the
	// kept World with large writes. Each stays well under the write buffer's
	// 4,096 entries: a larger one drains to its stage and is stored before
	// the checkpoint, which would leave the checkpoint little to store.
	var base []time.Duration
	for range 8 {
		base = append(base, write(1))
	}
	for range 20 {
		write(1000)
	}
	if above() >= sobject.MinCheckpointOperations {
		t.Fatalf("%d operations above the checkpoint before the timed one", above())
	}

	// Write until the driver owes a checkpoint, then time writes until it
	// lands: the first of them lands while it stores. The store takes from
	// under a second to several on a loaded machine, so the writes are paced
	// and bounded by time, which also bounds the operations above the
	// checkpoint.
	// A store fast enough to land before the first timed write leaves
	// nothing to time, so the phase runs again from the checkpoint it left.
	const pace, limit = 10 * time.Millisecond, 60 * time.Second
	tick := time.NewTicker(pace)
	defer tick.Stop()
	var during []time.Duration
	var stored time.Duration
	for attempt := 0; len(during) == 0; attempt++ {
		if attempt == 3 {
			t.Fatal("the checkpoint landed before any write timed during it, three times")
		}
		for above() < sobject.MinCheckpointOperations {
			write(1)
		}
		start := time.Now()
		for checkpointHeight(ctx, t, so) == height {
			if time.Since(start) > limit {
				t.Fatalf("the checkpoint did not land within %v", limit)
			}
			during = append(during, write(1))
			<-tick.C
		}
		stored = time.Since(start)
		height = checkpointHeight(ctx, t, so)
	}

	// The writes during it were decided about as fast as with none running.
	slices.Sort(base)
	baseMax, duringMax := base[len(base)-1], slices.Max(during)
	t.Logf("checkpoint landed after %v and %d writes; write decision with none running: median %v, max %v; during it: max %v",
		stored, len(during), base[len(base)/2], baseMax, duringMax)
	if duringMax > stored/4 {
		t.Fatalf("a write landing during a %v checkpoint was decided after %v", stored, duringMax)
	}
}
