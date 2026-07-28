//go:build !js

package resource_world_test

import (
	"context"
	"testing"
	"time"

	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/byteslice"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

type bodyPageTrackingRaceWorldState struct {
	world.WorldState

	live      world.WorldState
	engine    world.Engine
	objectKey string
	updated   bool
}

func (w *bodyPageTrackingRaceWorldState) GetObjectBodiesBatchPage(
	ctx context.Context,
	keys []string,
	byteBudget int,
) ([]*world.ObjectBody, uint32, error) {
	bodies, consumed, _, err := w.GetObjectBodiesBatchPageWithSeqno(ctx, keys, byteBudget)
	return bodies, consumed, err
}

func (w *bodyPageTrackingRaceWorldState) GetObjectBodiesBatchPageWithSeqno(
	ctx context.Context,
	keys []string,
	byteBudget int,
) ([]*world.ObjectBody, uint32, uint64, error) {
	bodies, consumed, seqno, err := world.GetObjectBodiesBatchPageWithSeqno(ctx, w.WorldState, keys, byteBudget)
	if err != nil {
		return nil, 0, 0, err
	}
	if !w.updated {
		w.updated = true
		err = w.commitObjectUpdate(ctx)
		if err != nil {
			return nil, 0, 0, err
		}
	}
	return bodies, consumed, seqno, nil
}

func (w *bodyPageTrackingRaceWorldState) GetObjectRootRefsBatch(ctx context.Context, keys []string) ([]*world.ObjectRootRef, error) {
	return world.GetObjectRootRefsBatch(ctx, w.live, keys)
}

func (w *bodyPageTrackingRaceWorldState) commitObjectUpdate(ctx context.Context) error {
	updateTx, err := w.engine.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer updateTx.Discard()

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

func TestWatchWorldStateBatchedBodyReadTracksAccess(t *testing.T) {
	ctx := context.Background()
	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()

	resClient, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
	defer cleanup()

	writeTx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction: %v", err)
	}
	objectKey := "batch-followup/revision"
	if _, err := writeTx.CreateObject(ctx, objectKey, nil); err != nil {
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

	trackedRef := resClient.CreateResourceReference(watchMsg.GetResourceId())
	defer trackedRef.Release()
	trackedClient, err := trackedRef.GetClient()
	if err != nil {
		t.Fatalf("tracked resource client: %v", err)
	}
	bodyService := s4wave_world.NewSRPCWorldStateResourceServiceClient(trackedClient)
	resp, err := bodyService.GetObjectBodiesBatch(ctx, &s4wave_world.GetObjectBodiesBatchRequest{
		ObjectKeys: []string{objectKey},
	})
	if err != nil {
		t.Fatalf("GetObjectBodiesBatch: %v", err)
	}
	if resp.GetWorldSeqno() == 0 {
		t.Fatal("GetObjectBodiesBatch returned zero world_seqno for nonzero revision")
	}
	updateTx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction update: %v", err)
	}
	defer updateTx.Release()
	obj, found, err := updateTx.GetObject(ctx, objectKey)
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

func TestWatchWorldStateBatchedBodyReadTracksPageSnapshotRevision(t *testing.T) {
	ctx := t.Context()
	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()

	const objectKey = "batch-followup/snapshot-revision"
	initialBody := []byte("version-before-page-read")
	writeTx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction initial: %v", err)
	}
	_, _, err = world.CreateWorldObject(ctx, writeTx, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(byteslice.NewByteSlice(&initialBody), true)
		return nil
	})
	if err != nil {
		writeTx.Discard()
		t.Fatalf("CreateWorldObject initial: %v", err)
	}
	if err := writeTx.Commit(ctx); err != nil {
		writeTx.Discard()
		t.Fatalf("Commit initial: %v", err)
	}
	writeTx.Discard()

	readTx, err := tb.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction read: %v", err)
	}
	defer readTx.Discard()
	seqno, err := readTx.GetSeqno(ctx)
	if err != nil {
		t.Fatalf("GetSeqno read: %v", err)
	}

	liveWs := world.NewEngineWorldState(tb.Engine, false)
	trackedCtx, trackedCancel := context.WithCancel(ctx)
	defer trackedCancel()
	trackedWs := resource_world.NewTrackedWorldState(
		&bodyPageTrackingRaceWorldState{
			WorldState: readTx,
			live:       liveWs,
			engine:     tb.Engine,
			objectKey:  objectKey,
		},
		liveWs,
		seqno,
		trackedCtx,
	)
	defer trackedWs.Close()

	bodies, _, _, err := trackedWs.GetObjectBodiesBatchPageWithSeqno(ctx, []string{objectKey}, world.ObjectBodiesBatchByteBudget)
	if err != nil {
		t.Fatalf("GetObjectBodiesBatchPageWithSeqno: %v", err)
	}
	if len(bodies) != 1 || string(bodies[0].Body) != string(initialBody) {
		t.Fatalf("body page = %+v, want initial body", bodies)
	}

	waitCtx, waitCancel := context.WithTimeout(ctx, time.Second)
	defer waitCancel()
	if err := trackedWs.WaitForChanges(waitCtx); err != nil {
		t.Fatalf("WaitForChanges after racing commit: %v", err)
	}
}
