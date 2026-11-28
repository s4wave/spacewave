//go:build !goscript

package space_world_test

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// TestSetRootDeferredDrained writes a transaction too large for its write
// buffer, whose World pages drain to the writer's stage before Admit, and
// stops any moves it queues before they land. The writer's stage ends with
// the transaction's retirement, so a collection then must still keep the
// pages, and the pending root the moves land must read back whole. Its 6000
// objects, collection and remount run far longer under GoScript than the
// package's CI budget allows, so it runs only under Go.
func TestSetRootDeferredDrained(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Second)
	defer cancel()
	defer space_world.ForceDeferredMoves()()
	f := newAdapterFixtureOn(t, fixtureOpts{frees: true, opTypes: &space_world.OpTypes{Lookup: optypes.LookupWorldOp}})
	eng, engRef := f.start(t)
	defer func() { engRef.Release() }()
	adapter := adapterOf(ctx, t, eng)

	// See the drained candidate, and stop its moves before they land.
	t.Setenv(world_block.DurabilityTimeoutEnv, "30s")
	var drained atomic.Bool
	defer space_world.SetAdmittedHook(func(c world.Candidate) {
		if c.Staged {
			drained.Store(true)
		}
	})()
	reached, resume := pauseFirstLanding(t)
	werr := make(chan error, 1)
	go func() {
		werr <- world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
			for _, key := range drainedKeys() {
				obj, err := ws.CreateObject(ctx, key, nil)
				world.ReleaseObjectState(obj)
				if err != nil {
					return err
				}
			}
			return nil
		})
	}()
	// The write either queues its moves, which stop, or lands them in its
	// turn and returns.
	landed := false
	select {
	case <-reached:
	case err := <-werr:
		if err != nil {
			t.Fatal(err)
		}
		landed = true
	case <-ctx.Done():
		t.Fatal("the write neither queued a root move nor returned")
	}

	// A new turn waits out the write's retirement, which ends its stage.
	turn, err := adapter.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	turn.Release()
	if !drained.Load() {
		t.Fatal("the write drained no World page to its stage")
	}

	// Free what nothing holds, then land the moves.
	collectDecoy(ctx, t, f)
	resume()
	if !landed {
		if err := <-werr; err != nil {
			t.Fatal(err)
		}
	}
	if err := adapter.Acknowledge(ctx); err != nil {
		t.Fatal(err)
	}

	// Pending holds the pages until an install covers the write, which may
	// already have moved them under the replay cursor, so walk pending only
	// while it still holds them. Then collect again, remount so reads come
	// from the store, and read every object: the written World must survive
	// whichever root holds it.
	store := f.space(t).GetBlockStore()
	root, err := store.GetRoot(ctx, space_world.RootPending)
	if err != nil {
		t.Fatal(err)
	}
	if !root.GetEmpty() {
		if err := block.WalkGraph(ctx, store, []*block.BlockRef{root}, 0, nil); err != nil {
			t.Fatalf("the pending root names a freed World page: %v", err)
		}
	}
	collectDecoy(ctx, t, f)
	eng, engRef = f.remount(ctx, t, eng, engRef)
	if err := hasObjects(ctx, eng, drainedKeys()...); err != nil {
		t.Fatalf("the drained write lost a World page: %v; %s", err, f.collectReport(ctx))
	}
}

// drainedKeys returns the keys TestSetRootDeferredDrained writes.
func drainedKeys() []string {
	keys := make([]string, 6000)
	for i := range keys {
		keys[i] = "drained-" + strconv.Itoa(i)
	}
	return keys
}
