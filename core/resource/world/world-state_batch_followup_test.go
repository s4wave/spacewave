//go:build !js

package resource_world_test

import (
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/byteslice"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// bodyPageRaceWorldState commits a change to objectKey once, right after the
// first body page read returns.
type bodyPageRaceWorldState struct {
	world.WorldState

	engine    world.Engine
	objectKey string
	updated   bool
}

// GetObjectBodiesBatchPage reads one body page with its seqno, then commits the
// change once.
func (w *bodyPageRaceWorldState) GetObjectBodiesBatchPage(
	ctx context.Context,
	keys []string,
	byteBudget int,
) ([]*world.ObjectBody, uint32, uint64, error) {
	bodies, consumed, seqno, err := w.WorldState.GetObjectBodiesBatchPage(ctx, keys, byteBudget)
	if err != nil {
		return nil, 0, 0, err
	}
	if !w.updated {
		w.updated = true
		if err := w.commitObjectUpdate(ctx); err != nil {
			return nil, 0, 0, err
		}
	}
	return bodies, consumed, seqno, nil
}

// commitObjectUpdate commits a new body for objectKey.
func (w *bodyPageRaceWorldState) commitObjectUpdate(ctx context.Context) error {
	// Open a write transaction for the change.
	updateTx, err := w.engine.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer updateTx.Discard()

	// Replace the body and commit.
	body := []byte("version-after-page-read")
	_, _, err = world.AccessWorldObject(ctx, updateTx, w.objectKey, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(byteslice.NewByteSlice(&body), true)
		return nil
	})
	if err != nil {
		return err
	}
	return updateTx.Commit(ctx)
}

// TestWatchWorldStateBatchedBodyReadTracksAccess checks that a body read
// through the batched body stream of a watched snapshot republishes the
// snapshot when the object changes.
func TestWatchWorldStateBatchedBodyReadTracksAccess(t *testing.T) {
	// Serve the World over a resource client.
	ctx := context.Background()
	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()
	resClient, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
	defer cleanup()

	// Create the object.
	writeTx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction: %v", err)
	}
	objectKey := "batch-followup/revision"
	createdObject, err := writeTx.CreateObject(ctx, objectKey, nil)
	world.ReleaseObjectState(createdObject)
	if err != nil {
		writeTx.Release()
		t.Fatalf("CreateObject: %v", err)
	}
	if err := writeTx.Commit(ctx); err != nil {
		writeTx.Release()
		t.Fatalf("Commit: %v", err)
	}
	writeTx.Release()

	// The stream stays open across the batched read and the update transaction
	// below, so a deadline here would be a budget over that setup work rather
	// than a bound on any wait. Each awaited message is bounded on its own.
	stream, err := engine.WatchWorldState(ctx)
	if err != nil {
		t.Fatalf("WatchWorldState: %v", err)
	}
	defer stream.Close()
	watchMsg := recvWatchWorldState(t, stream, "initial snapshot")

	// Open the watched snapshot.
	trackedRef := resClient.CreateResourceReference(watchMsg.GetResourceId())
	defer trackedRef.Release()
	trackedClient, err := trackedRef.GetClient()
	if err != nil {
		t.Fatalf("tracked resource client: %v", err)
	}

	// Read the body through the snapshot's batched body stream.
	bodyService := s4wave_world.NewSRPCWorldStateResourceServiceClient(trackedClient)
	bodyStream, err := bodyService.GetObjectBodiesBatch(ctx, &s4wave_world.GetObjectBodiesBatchRequest{
		ObjectKeys: []string{objectKey},
	})
	if err != nil {
		t.Fatalf("GetObjectBodiesBatch: %v", err)
	}
	resp, err := bodyStream.Recv()
	bodyStream.Close()
	if err != nil {
		t.Fatalf("GetObjectBodiesBatch page: %v", err)
	}
	if resp.GetWorldSeqno() == 0 {
		t.Fatal("GetObjectBodiesBatch returned zero world_seqno for nonzero revision")
	}

	// Change the object.
	updateTx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction update: %v", err)
	}
	defer updateTx.Release()

	// Bump its revision and commit.
	obj, found, err := updateTx.GetObject(ctx, objectKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatalf("GetObject update: %v", err)
	}
	if !found {
		t.Fatalf("object %q missing during update", objectKey)
	}
	if _, err := obj.IncrementRev(ctx); err != nil {
		t.Fatalf("IncrementRev update: %v", err)
	}
	if err := updateTx.Commit(ctx); err != nil {
		t.Fatalf("Commit update: %v", err)
	}

	// Expect a new snapshot.
	changed := recvWatchWorldState(t, stream, "update after the batched body read and commit")
	if changed.GetResourceId() == watchMsg.GetResourceId() {
		t.Fatal("batched body access did not track object changes")
	}
}

// recvWatchWorldState waits for the next message on a world state watch stream
// under its own bound.
//
// Recv takes no context, so the only deadline lever is the stream context, and
// that context has to outlive the whole flow the caller is exercising. Bounding
// the stream would therefore budget the setup rather than the wait. Each awaited
// message gets its own bound here instead, so a stream that never publishes
// fails at the receive that stalled rather than hanging the package.
func recvWatchWorldState(
	t *testing.T,
	stream s4wave_world.SRPCWatchWorldStateResourceService_WatchWorldStateClient,
	what string,
) *s4wave_world.WatchWorldStateResponse {
	// Receive on its own goroutine so the wait can time out.
	t.Helper()
	type watchRecv struct {
		msg *s4wave_world.WatchWorldStateResponse
		err error
	}
	recvCh := make(chan watchRecv, 1)
	go func() {
		msg, err := stream.Recv()
		recvCh <- watchRecv{msg: msg, err: err}
	}()
	select {
	case recv := <-recvCh:
		if recv.err != nil {
			t.Fatalf("WatchWorldState Recv (%s): %v", what, recv.err)
		}
		return recv.msg
	case <-time.After(5 * time.Second):
		t.Fatalf("no world state %s arrived", what)
		return nil
	}
}

// TestReadSetBatchedBodyReadSeesRacingCommit checks that a commit landing
// while a tracked body page is read still ends the wait that follows.
func TestReadSetBatchedBodyReadSeesRacingCommit(t *testing.T) {
	// Open a World.
	ctx := t.Context()
	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()

	// Create the object with its initial body.
	const objectKey = "batch-followup/snapshot-revision"
	initialBody := []byte("version-before-page-read")
	writeTx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction initial: %v", err)
	}

	// Write the body and commit.
	createdObject, _, err := world.CreateWorldObject(ctx, writeTx, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(byteslice.NewByteSlice(&initialBody), true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		writeTx.Discard()
		t.Fatalf("CreateWorldObject initial: %v", err)
	}
	if err := writeTx.Commit(ctx); err != nil {
		writeTx.Discard()
		t.Fatalf("Commit initial: %v", err)
	}
	writeTx.Discard()

	// Open the snapshot the view reads.
	readTx, err := tb.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction read: %v", err)
	}
	defer readTx.Discard()
	seqno, err := readTx.GetSeqno(ctx)
	if err != nil {
		t.Fatalf("GetSeqno read: %v", err)
	}

	// Read the body page through the tracked state while a commit changes it.
	reads := world.NewReadSet()
	tracked := reads.Track(&bodyPageRaceWorldState{
		WorldState: readTx,
		engine:     tb.Engine,
		objectKey:  objectKey,
	})
	bodies, _, _, err := tracked.GetObjectBodiesBatchPage(ctx, []string{objectKey}, world.ObjectBodiesBatchByteBudget)
	if err != nil {
		t.Fatalf("GetObjectBodiesBatchPage: %v", err)
	}
	if len(bodies) != 1 || string(bodies[0].Body) != string(initialBody) {
		t.Fatalf("body page = %+v, want initial body", bodies)
	}

	// Expect the wait from the snapshot to see the commit.
	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	if _, err := reads.WaitChange(waitCtx, tb.Engine, seqno); err != nil {
		t.Fatalf("WaitChange after racing commit: %v", err)
	}
}
