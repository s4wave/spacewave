//go:build !js

package resource_world_test

import (
	"context"
	"testing"
	"time"

	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/db/world"
)

// commitDuringCheckWorldState commits one change to objectKey from inside the
// tracked-revision check, after the check has already read the revisions.
//
// That places the commit in the window between the revision check and the
// seqno the watch waits on, which is the ordering a loaded runner produces on
// its own when the watch routine is descheduled between those two reads.
type commitDuringCheckWorldState struct {
	world.WorldState

	engine    world.Engine
	objectKey string
	committed bool
}

func (w *commitDuringCheckWorldState) GetObjectRootRefsBatch(ctx context.Context, keys []string) ([]*world.ObjectRootRef, error) {
	refs, err := world.GetObjectRootRefsBatch(ctx, w.WorldState, keys)
	if err != nil {
		return nil, err
	}
	if !w.committed {
		w.committed = true
		if err := w.commitObjectRevision(ctx); err != nil {
			return nil, err
		}
	}
	return refs, nil
}

func (w *commitDuringCheckWorldState) commitObjectRevision(ctx context.Context) error {

	// newTransaction tx,err.
	tx, err := w.engine.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// getObject obj,found,err via tx.
	obj, found, err := tx.GetObject(ctx, w.objectKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	if !found {
		return world.ErrObjectNotFound
	}
	if _, err := obj.IncrementRev(ctx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func TestTrackedWorldStateCommitDuringRevisionCheck(t *testing.T) {

	// context ctx.
	ctx := t.Context()
	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()

	// Perform the action.
	const objectKey = "tracked-watch/commit-during-check"
	writeTx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction initial: %v", err)
	}
	{
		createdObject, err := writeTx.CreateObject(ctx, objectKey, nil)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			writeTx.Discard()
			t.Fatalf("CreateObject initial: %v", err)
		}
	}
	if err := writeTx.Commit(ctx); err != nil {
		writeTx.Discard()
		t.Fatalf("Commit initial: %v", err)
	}
	writeTx.Discard()

	// newTransaction readTx,err via tb.
	readTx, err := tb.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction read: %v", err)
	}
	defer readTx.Discard()
	seqno, err := readTx.GetSeqno(ctx)
	if err != nil {
		t.Fatalf("GetSeqno read: %v", err)
	}

	// Record watchWs.
	watchWs := &commitDuringCheckWorldState{
		WorldState: world.NewEngineWorldState(tb.Engine, false),
		engine:     tb.Engine,
		objectKey:  objectKey,
	}

	// withCancel trackedCtx,trackedCancel via context.
	trackedCtx, trackedCancel := context.WithCancel(ctx)
	defer trackedCancel()
	trackedWs := resource_world.NewTrackedWorldState(readTx, watchWs, seqno, trackedCtx)
	defer trackedWs.Close()

	// Perform the action.
	{
		objectState, _, err := trackedWs.GetObject(ctx, objectKey)
		world.ReleaseObjectState(objectState)
		if err != nil {
			t.Fatalf("GetObject tracked: %v", err)
		}
	}

	// withTimeout waitCtx,waitCancel via context.
	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	if err := trackedWs.WaitForChanges(waitCtx); err != nil {
		t.Fatalf("WaitForChanges after commit racing the revision check: %v", err)
	}
}
