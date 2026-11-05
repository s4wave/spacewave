package space_world_test

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// rootSetOp encodes a device's answer to fence with roots.
func rootSetOp(t *testing.T, fence []byte, roots ...*block.BlockRef) []byte {
	t.Helper()
	op, err := (&space_world.WorldOp{Body: &space_world.WorldOp_RootSet{RootSet: &space_world.RootSetOp{Fence: fence, Roots: roots}}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return op
}

// testRef returns the reference of a block with data, which the fence only
// collects.
func testRef(t *testing.T, data string) *block.BlockRef {
	t.Helper()
	ref, err := block.BuildBlockRef([]byte(data), nil)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// waitFence waits for a fence other than skip in so's order and returns its
// hash.
func waitFence(ctx context.Context, t *testing.T, so sobject.SharedObject, skip []byte) []byte {
	t.Helper()
	ctr, rel, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	var fence []byte
	_, err = ctr.WaitValueWithValidator(ctx, func(snap sobject.SharedObjectStateSnapshot) (bool, error) {
		if snap == nil {
			return false, nil
		}
		set, err := snap.GetOperationSet(ctx)
		if err != nil {
			return false, err
		}
		for _, h := range set.Order() {
			data, err := snap.DecodeOperation(ctx, set.Get(h))
			if err != nil || len(data) == 0 || bytes.Equal(h, skip) {
				continue
			}
			op := &space_world.WorldOp{}
			if op.UnmarshalVT(data) == nil && op.GetFence() != nil {
				fence = h
				return true, nil
			}
		}
		return false, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return fence
}

// TestAdapterStorageFence checks the storage fence against the ways it could
// let a pass delete a live block: it waits for every writer, the trimming
// roster's dropped ones included; an answer to an earlier fence does not
// count; a device that joins after the fence does not hold the pass; and a
// writer that never answers fails the pass, which then returns no roots.
func TestAdapterStorageFence(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	f := newAdapterFixture(t)
	_, engRef := f.start(t)
	defer engRef.Release()
	busEngine := world.NewBusEngine(ctx, f.tb.Bus, f.engineID)
	if err := world.ExecTransaction(ctx, busEngine, true, func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, "fenced", nil)
		world.ReleaseObjectState(obj)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var adapter *space_world.Adapter
	if err := world.AccessEngine(ctx, busEngine, func(_ context.Context, eng world.Engine) error {
		adapter = eng.(*space_world.Engine).GetAdapter()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	so := f.space(t)
	d1, d2 := addDevice(t, so), addDevice(t, so)

	// Run a pass's fence.
	type result struct {
		live []*block.BlockRef
		err  error
	}
	done := make(chan result, 1)
	go func() {
		live, err := adapter.ReclaimFence(ctx, false)
		done <- result{live, err}
	}()
	fence := waitFence(ctx, t, so, nil)
	pending := func(what string) {
		t.Helper()
		select {
		case r := <-done:
			t.Fatalf("fence returned %v, %v %s", r.live, r.err, what)
		case <-time.After(200 * time.Millisecond):
		}
	}

	// One writer's answer, a stale answer from the other, and a device that
	// joined after the fence leave it waiting.
	r1, r2 := testRef(t, "device one root"), testRef(t, "device two root")
	d1.write(t, so, rootSetOp(t, fence, r1), false, false)
	pending("with one writer unanswered")
	d2.write(t, so, rootSetOp(t, testRef(t, "an earlier fence").GetHash().GetHash(), r2), false, false)
	pending("with a stale answer")
	addDevice(t, so)

	// The other writer's answer completes it with every writer's roots and
	// this device's own.
	d2.write(t, so, rootSetOp(t, fence, r2), false, false)
	r := <-done
	if r.err != nil {
		t.Fatal(r.err)
	}
	base, err := so.GetBlockStore().GetRoot(ctx, space_world.RootReplayBase)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []*block.BlockRef{r1, r2, base} {
		if !want.GetEmpty() && !slices.ContainsFunc(r.live, want.EqualsRef) {
			t.Fatalf("live roots %v lack %s", r.live, want.MarshalString())
		}
	}

	// A writer that never answers fails the next pass.
	short, cancelShort := context.WithTimeout(ctx, 2*time.Second)
	defer cancelShort()
	go func() {
		live, err := adapter.ReclaimFence(short, false)
		done <- result{live, err}
	}()
	next := waitFence(ctx, t, so, fence)
	d1.write(t, so, rootSetOp(t, next, r1), false, false)
	if r := <-done; r.err == nil || r.live != nil {
		t.Fatalf("pass with an unanswered writer returned %v, %v; want an error and no roots", r.live, r.err)
	}
}
