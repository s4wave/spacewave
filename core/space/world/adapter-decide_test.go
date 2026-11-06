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

	// Write until a first checkpoint lands, so the next counts from it.
	for checkpointHeight(ctx, t, so) == 0 {
		write(1)
	}
	waitCheckpointAbove(ctx, t, so, 0)
	height := checkpointHeight(ctx, t, so)

	// Time single-object writes with no checkpoint running, then grow the
	// kept World with large writes.
	var base []time.Duration
	for range 8 {
		base = append(base, write(1))
	}
	for range 4 {
		write(5000)
	}
	if above() >= sobject.MinCheckpointOperations {
		t.Fatalf("%d operations above the checkpoint before the timed one", above())
	}

	// Write until the driver owes a checkpoint, then time writes until it
	// lands: the first of them lands while it stores.
	for above() < sobject.MinCheckpointOperations {
		write(1)
	}
	start := time.Now()
	var during []time.Duration
	for checkpointHeight(ctx, t, so) == height {
		during = append(during, write(1))
		if len(during) == 500 {
			t.Fatal("the checkpoint did not land within 500 writes")
		}
	}
	stored := time.Since(start)

	// The writes during it were decided about as fast as with none running.
	slices.Sort(base)
	baseMax, duringMax := base[len(base)-1], slices.Max(during)
	t.Logf("checkpoint landed after %v and %d writes; write decision with none running: median %v, max %v; during it: max %v",
		stored, len(during), base[len(base)/2], baseMax, duringMax)
	if duringMax > stored/4 {
		t.Fatalf("a write landing during a %v checkpoint was decided after %v", stored, duringMax)
	}
}
