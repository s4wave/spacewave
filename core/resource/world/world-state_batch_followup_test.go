//go:build !js

package resource_world_test

import (
	"context"
	"testing"
	"time"

	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

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

	watchCtx, watchCancel := context.WithTimeout(ctx, 3*time.Second)
	defer watchCancel()
	stream, err := engine.WatchWorldState(watchCtx)
	if err != nil {
		t.Fatalf("WatchWorldState: %v", err)
	}
	defer stream.Close()
	watchMsg, err := stream.Recv()
	if err != nil {
		t.Fatalf("WatchWorldState Recv: %v", err)
	}

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

	changedMsg, err := stream.Recv()
	if err != nil {
		t.Fatalf("WatchWorldState Recv after batched access: %v", err)
	}
	if changedMsg.GetResourceId() == watchMsg.GetResourceId() {
		t.Fatal("batched body access did not track object changes")
	}
}
