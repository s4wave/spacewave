package sobject_world_engine

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
)

// TestWatchStateKeepsAcceptedWorldBase rejects speculative publication before
// the queued operation has an accepted SharedObject root.
func TestWatchStateKeepsAcceptedWorldBase(t *testing.T) {
	// Queue a real object creation without advancing the accepted root.
	ctx := t.Context()
	c, so, head := newProcessTestWorld(t, ctx)
	ws, err := c.buildBlkEngine(ctx, c.le, so, head.GetHeadRef().CloneVT(), head.GetHeadRef().GetTransformConf())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ws.Release)
	op, err := world_block_tx.NewTxCreateObject("pending-object", head.GetHeadRef())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := &pendingWorldSnapshot{
		testFinalizationSnapshot: newTestFinalizationSnapshot(t, &sobject.SORoot{InnerSeqno: 1}, head.GetHeadRef()),
		pending:                  []*sobject.QueuedSOOperation{{LocalId: "pending", OpData: marshalApplyTxOpForProcessTest(t, op)}},
	}
	shared := &transactionSharedObject{
		testSharedObject: *so,
		snapshot:         snapshot,
	}
	engine := newSoEngine(c, shared, ws.bengine)

	// The watcher must leave the transaction base acceptable to finalization.
	if err := c.executeWatchSOStateOnce(ctx, snapshot, engine); err != nil {
		t.Fatal(err)
	}
	packet := newTestFinalizationPacket(t, snapshot.root, engine.bengine.GetRootRef(), head.GetHeadRef(), "next")
	if err := engine.validateFinalizationBase(ctx, packet); err != nil {
		t.Fatalf("watcher installed an unaccepted transaction base: %v", err)
	}
	read, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	if found, err := read.HasObject(ctx, "pending-object"); err != nil || found {
		t.Fatalf("pending object visible before acceptance: found=%v, err=%v", found, err)
	}
}

// TestWriteTransactionRefreshesAcceptedBase checks that a lagging watcher cannot
// make the first write after a remote acceptance use an older World.
func TestWriteTransactionRefreshesAcceptedBase(t *testing.T) {
	// Advance authority while leaving the local engine on its previous root.
	ctx := t.Context()
	c, so, head := newProcessTestWorld(t, ctx)
	ws, err := c.buildBlkEngine(ctx, c.le, so, head.GetHeadRef().CloneVT(), head.GetHeadRef().GetTransformConf())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ws.Release)
	accepted := applyTransactionTestObject(t, c, so, head, "remote-object")
	shared := &transactionSharedObject{
		testSharedObject: *so,
		snapshot:         newTestFinalizationSnapshot(t, &sobject.SORoot{InnerSeqno: 2}, accepted.GetHeadRef()),
	}
	engine := newSoEngine(c, shared, ws.bengine)

	// The write reads accepted remote data even before the watcher runs.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if found, err := tx.HasObject(ctx, "remote-object"); err != nil || !found {
		t.Fatalf("write did not refresh accepted state: found=%v, err=%v", found, err)
	}
	{
		createdObject, err := tx.CreateObject(ctx, "local-object", head.GetHeadRef())
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Accept the submitted operation through replay, preserving the remote write.
	shared.afterWait = func() {
		if len(shared.queuedOps) != 1 {
			t.Fatalf("queued operations = %d, want 1", len(shared.queuedOps))
		}
		next, result, err := c.processOp(ctx, c.le, shared, shared.queuedOps[0], "local", newProcessTestPeerID(t), 2, 0, accepted)
		if err != nil {
			t.Fatal(err)
		}
		if !result.GetSuccess() {
			t.Fatalf("local transaction rejected: %v", result)
		}
		shared.snapshot = newTestFinalizationSnapshot(t, &sobject.SORoot{InnerSeqno: 3}, next.GetHeadRef())
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit after refreshing the accepted base: %v", err)
	}

	// Read back both writes through the engine's newly accepted head.
	read, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	for _, key := range []string{"remote-object", "local-object"} {
		if found, err := read.HasObject(ctx, key); err != nil || !found {
			t.Fatalf("accepted object %q: found=%v, err=%v", key, found, err)
		}
	}
}

// TestWriteTransactionRetainsSharedObjectBase rejects an authority change during
// the transaction even when that change leaves the World head unchanged.
func TestWriteTransactionRetainsSharedObjectBase(t *testing.T) {
	// Open a write under one authority root.
	ctx := t.Context()
	c, so, head := newProcessTestWorld(t, ctx)
	ws, err := c.buildBlkEngine(ctx, c.le, so, head.GetHeadRef().CloneVT(), head.GetHeadRef().GetTransformConf())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ws.Release)
	shared := &transactionSharedObject{
		testSharedObject: *so,
		snapshot:         newTestFinalizationSnapshot(t, &sobject.SORoot{InnerSeqno: 1}, head.GetHeadRef()),
	}
	engine := newSoEngine(c, shared, ws.bengine)
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	{
		createdObject, err := tx.CreateObject(ctx, "local-object", head.GetHeadRef())
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Change authority before commit and require rejection before submission.
	shared.snapshot = newTestFinalizationSnapshot(t, &sobject.SORoot{InnerSeqno: 2}, head.GetHeadRef())
	if err := tx.Commit(ctx); !errors.Is(err, coord.ErrStaleGeneration) {
		t.Fatalf("authority changed during write: got %v, want stale generation", err)
	}
	if len(shared.queuedOps) != 0 {
		t.Fatal("stale transaction reached the authority queue")
	}
}

// applyTransactionTestObject creates an accepted World head through real replay.
func applyTransactionTestObject(t *testing.T, c *Controller, so sobject.SharedObject, head *InnerState, key string) *InnerState {
	t.Helper()
	op, err := world_block_tx.NewTxCreateObject(key, head.GetHeadRef())
	if err != nil {
		t.Fatal(err)
	}
	next, result, err := c.processOp(t.Context(), c.le, so, marshalApplyTxOpForProcessTest(t, op), key, newProcessTestPeerID(t), 1, 0, head)
	if err != nil {
		t.Fatal(err)
	}
	if !result.GetSuccess() {
		t.Fatalf("create object rejected: %v", result)
	}
	return next
}

// pendingWorldSnapshot supplies a queued operation separately from accepted state.
type pendingWorldSnapshot struct {
	*testFinalizationSnapshot
	// pending contains operations awaiting authority acceptance.
	pending []*sobject.QueuedSOOperation
}

// GetOpQueue returns the operations not yet reflected in the accepted World.
func (s *pendingWorldSnapshot) GetOpQueue(context.Context) ([]*sobject.SOOperation, []*sobject.QueuedSOOperation, error) {
	return nil, s.pending, nil
}

// transactionSharedObject controls authority advancement while real World storage
// and replay run unchanged. This makes watcher lag deterministic without timers.
type transactionSharedObject struct {
	testFinalizationSharedObject
	// snapshot is the current authority state, independent of watcher progress.
	snapshot sobject.SharedObjectStateSnapshot
}

// GetSharedObjectState returns the latest accepted authority snapshot.
func (s *transactionSharedObject) GetSharedObjectState(context.Context) (sobject.SharedObjectStateSnapshot, error) {
	return s.snapshot, nil
}

// _ is a type assertion
var _ sobject.SharedObject = (*transactionSharedObject)(nil)
