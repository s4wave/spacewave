//go:build !goscript

package space_world_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/world"
)

// TestAdapterDecidesWhileCheckpointStores checks that the driver decides and
// publishes a write even when checkpoint page storage cannot finish.
func TestAdapterDecidesWhileCheckpointStores(t *testing.T) {
	// Serve a Space whose first checkpoint is still below its trim threshold.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	f := newAdapterFixture(t)
	_, engRef := f.start(t)
	defer engRef.Release()
	busEngine := world.NewBusEngine(ctx, f.tb.Bus, f.engineID)
	so := f.space(t)
	var next int
	write := func() string {
		t.Helper()
		key := "decide-" + strconv.Itoa(next)
		next++
		if err := world.ExecTransaction(ctx, busEngine, true, func(ctx context.Context, ws world.WorldState) error {
			obj, err := ws.CreateObject(ctx, key, nil)
			world.ReleaseObjectState(obj)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return key
	}

	// Park the checkpoint before the page store takes the root lock. Either
	// cleanup or cancellation can release the store before engine teardown.
	reached, release := make(chan struct{}), make(chan struct{})
	entered := sync.OnceFunc(func() { close(reached) })
	resume := sync.OnceFunc(func() { close(release) })
	defer space_world.SetCheckpointStoreHook(func(ctx context.Context) {
		entered()
		select {
		case <-release:
		case <-ctx.Done():
		}
	})()
	defer resume()

	// Fill the first stable prefix to the checkpoint threshold.
	for range sobject.MinCheckpointOperations {
		write()
	}
	select {
	case <-reached:
	case <-ctx.Done():
		t.Fatal("checkpoint store did not reach its gate")
	}

	// Decide and read back a later write before releasing checkpoint storage.
	height := checkpointHeight(ctx, t, so)
	key := write()
	if err := hasObjects(ctx, busEngine, key); err != nil {
		t.Fatalf("write decided but was not visible while checkpoint storage was parked: %v", err)
	}
	if got := checkpointHeight(ctx, t, so); got != height {
		t.Fatalf("checkpoint advanced from %d to %d while its store was parked", height, got)
	}

	// Let the held checkpoint land and leave its successor visible.
	resume()
	waitCheckpointAbove(ctx, t, so, height)
	if err := hasObjects(ctx, busEngine, key); err != nil {
		t.Fatalf("write was lost when the checkpoint landed: %v", err)
	}
	t.Log("write decided and read back before the parked checkpoint landed")
}
