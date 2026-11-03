//go:build !js

package sobject_world_engine_test

import (
	"context"
	"sync"
	"testing"
	"time"

	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
)

// TestBackfillStartupSnapshot restarts a real local World before its first
// background ownership update, copies it while writes continue, cancels that
// copy through controller retirement, and backfills its restored successor.
func TestBackfillStartupSnapshot(t *testing.T) {
	// Persist a payload so the restored copy performs actual graph reads.
	ctx := t.Context()
	w, so := startGatedWorld(t, t.TempDir(), nil)
	w.createWithPayload(t, "backfill/payload")
	so.stop()

	// Delay only post-startup replay watching and the independent graph copy.
	waitReplay, replayReached, releaseReplay := backfillGate()
	defer releaseReplay()
	waitCopy, copyReached, releaseCopy := backfillGate()
	defer releaseCopy()
	so.mtx.Lock()
	so.waitReplay, so.waitCopy = waitReplay, waitCopy
	so.mtx.Unlock()

	// Serve the restored World while its initial state watcher is paused.
	so.serve(t)
	var err error
	w.eng, err = so.ctrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err)
	}
	waitReached(t, replayReached)

	// The public head must describe the actual read despite uninitialized
	// background retention. Its root and sequence come from one snapshot.
	head, seqno := readBackfillHead(t, w)
	be := w.eng.(sobject_world_engine.BackfillEngine)
	got, gotSeqno, err := be.GetHead(ctx)
	if err != nil || !got.EqualsRef(head) || gotSeqno != seqno || head.GetEmpty() {
		t.Fatalf("startup head = %s/%d, %v; read = %s/%d", got.MarshalString(), gotSeqno, err, head.MarshalString(), seqno)
	}
	if err := be.SetBackfill(ctx, true); err != nil {
		t.Fatal(err)
	}
	waitReached(t, copyReached)

	// A blocked copy holds no foreground writer. The newer World survives
	// cancellation of the old copy and the controller's snapshot teardown.
	w.create(t, "backfill/while-copy")
	stopped := make(chan struct{})
	go func() {
		so.stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("controller teardown waited on the canceled backfill read")
	}
	releaseCopy()
	releaseReplay()

	// Restart without gates; the persisted backfill choice copies the new
	// World and publishes its actual root, not an empty startup completion.
	so.mtx.Lock()
	so.waitReplay, so.waitCopy = nil, nil
	so.mtx.Unlock()
	so.serve(t)
	w.eng, err = so.ctrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// The replacement's first copy describes its restored read snapshot.
	be = w.eng.(sobject_world_engine.BackfillEngine)
	state, err := be.GetBackfillState().WaitValue(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	head, seqno = readBackfillHead(t, w)
	if !state.IsComplete() || !state.GetRoot().EqualsRef(head) || state.GetSeqno() != seqno {
		t.Fatalf("restored backfill = %v; read = %s/%d", state, head.MarshalString(), seqno)
	}

	// Both foreground objects remain readable after the copy was canceled.
	read := newRead(t, w)
	for _, key := range []string{"backfill/payload", "backfill/while-copy"} {
		if !worldHas(t, read, key) {
			t.Fatalf("%s missing after controller restart", key)
		}
	}
	read.Discard()

	// An admission wakes backfill before its receipt is durable. With no
	// later write, settling that receipt must still copy the admitted World.
	applying, releasePublication := so.hold(nil)
	defer releasePublication()
	admission, err := world.CommitAdmit(ctx, w.newWrite(t, "backfill/pending"))
	if err != nil {
		t.Fatal(err)
	}
	defer admission.Release()
	waitReached(t, applying)

	// Complete the receipt without another write to wake a stale copy.
	releasePublication()
	if _, err := admission.WaitDurable(ctx); err != nil {
		t.Fatal(err)
	}
	head, seqno = readBackfillHead(t, w)
	_, err = be.GetBackfillState().WaitValueWithValidator(ctx, func(next *sobject_world_engine.BackfillState) (bool, error) {
		return next.IsComplete() && next.GetRoot().EqualsRef(head) && next.GetSeqno() == seqno, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

// readBackfillHead reads the actual immutable World, independent of GetHead.
func readBackfillHead(t *testing.T, w *localWorld) (*block.BlockRef, uint64) {
	// Retain the same snapshot for both root and World sequence.
	t.Helper()
	read := newRead(t, w)
	defer read.Discard()
	var head *block.BlockRef
	if err := read.AccessWorldState(t.Context(), nil, func(cursor *bucket_lookup.Cursor) error {
		head = cursor.GetRef().GetRootRef().CloneVT()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	seqno, err := read.GetSeqno(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return head, seqno
}

// backfillGate pauses an actual dependency until release or cancellation.
func backfillGate() (func(context.Context) error, <-chan struct{}, func()) {
	// Signal the first arrival once, including concurrent graph-copy reads.
	reached, released := make(chan struct{}), make(chan struct{})
	arrive := sync.OnceFunc(func() { close(reached) })
	release := sync.OnceFunc(func() { close(released) })
	wait := func(ctx context.Context) error {
		arrive()
		select {
		case <-released:
			return nil
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
	return wait, reached, release
}
