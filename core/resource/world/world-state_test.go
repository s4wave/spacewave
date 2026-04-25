package resource_world_test

import (
	"context"
	"fmt"
	"io"
	"testing"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	"github.com/s4wave/spacewave/db/bucket"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// setupWorldTestbed creates a hydra world testbed and returns it.
func setupWorldTestbed(ctx context.Context, t *testing.T) (*world_testbed.Testbed, func()) {
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	cleanup := func() {
		tb.Release()
	}

	return tb, cleanup
}

// setupWorldResourceClient sets up resource client and returns Engine SDK wrapper.
func setupWorldResourceClient(ctx context.Context, t *testing.T, tb *world_testbed.Testbed) (*resource_client.Client, *s4wave_world.Engine, func()) {
	resClient, clientCleanup := resource_testbed.SetupResourceClient(ctx, t, tb)

	rootRef := resClient.AccessRootResource()

	srpcClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		clientCleanup()
		t.Fatal(err.Error())
	}

	testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
	createWorldResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
	if err != nil {
		rootRef.Release()
		clientCleanup()
		t.Fatal(err.Error())
	}

	engineRef := resClient.CreateResourceReference(createWorldResp.ResourceId)
	engine, err := s4wave_world.NewEngine(resClient, engineRef)
	if err != nil {
		rootRef.Release()
		clientCleanup()
		t.Fatal(err.Error())
	}

	cleanup := func() {
		engine.Release()
		rootRef.Release()
		clientCleanup()
	}

	return resClient, engine, cleanup
}

// TestWorldStateBasicOperations tests basic WorldState operations using the SDK.
func TestWorldStateBasicOperations(t *testing.T) {
	ctx := context.Background()

	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()

	t.Run("CreateAndGetObject", func(t *testing.T) {
		_, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer tx.Release()

		objKey := "test-object-" + t.Name()
		rootRef := &bucket.ObjectRef{}

		obj, err := tx.CreateObject(ctx, objKey, rootRef)
		if err != nil {
			t.Fatalf("CreateObject failed: %v", err)
		}

		key := obj.GetKey()
		if key != objKey {
			t.Fatalf("expected key %q, got %q", objKey, key)
		}

		retrievedObj, found, err := tx.GetObject(ctx, objKey)
		if err != nil {
			t.Fatalf("GetObject failed: %v", err)
		}
		if !found {
			t.Fatal("object not found")
		}

		retrievedKey := retrievedObj.GetKey()
		if retrievedKey != objKey {
			t.Fatalf("expected retrieved key %q, got %q", objKey, retrievedKey)
		}

		t.Logf("Successfully created and retrieved object with key: %s", objKey)
	})

	t.Run("GetNonexistentObject", func(t *testing.T) {
		_, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer tx.Release()

		obj, found, err := tx.GetObject(ctx, "nonexistent-key")
		if err != nil {
			t.Fatalf("GetObject failed: %v", err)
		}
		if found {
			t.Fatal("expected object not found, but found=true")
		}
		if obj != nil {
			t.Fatal("expected nil object for not found")
		}

		t.Log("Correctly returned not found for nonexistent object")
	})

	t.Run("DeleteObject", func(t *testing.T) {
		_, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer tx.Release()

		objKey := "test-delete-" + t.Name()
		rootRef := &bucket.ObjectRef{}

		_, err = tx.CreateObject(ctx, objKey, rootRef)
		if err != nil {
			t.Fatalf("CreateObject failed: %v", err)
		}

		deleted, err := tx.DeleteObject(ctx, objKey)
		if err != nil {
			t.Fatalf("DeleteObject failed: %v", err)
		}
		if !deleted {
			t.Fatal("expected deleted=true")
		}

		_, found, err := tx.GetObject(ctx, objKey)
		if err != nil {
			t.Fatalf("GetObject after delete failed: %v", err)
		}
		if found {
			t.Fatal("expected object not found after delete")
		}

		t.Logf("Successfully deleted object: %s", objKey)
	})

	t.Run("GetReadOnly", func(t *testing.T) {
		_, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer tx.Release()

		readOnly := tx.GetReadOnly()
		if readOnly {
			t.Fatal("expected read-write transaction, got read-only")
		}

		t.Log("Correctly returned read-only status")
	})

	t.Run("GetSeqno", func(t *testing.T) {
		_, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer tx.Release()

		seqno, err := tx.GetSeqno(ctx)
		if err != nil {
			t.Fatalf("GetSeqno failed: %v", err)
		}

		t.Logf("Current seqno: %d", seqno)
	})

	t.Run("BuildStorageCursorOnTx", func(t *testing.T) {
		resClient, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		tx, err := engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer tx.Release()

		cursorResourceId, err := tx.BuildStorageCursor(ctx)
		if err != nil {
			t.Fatalf("BuildStorageCursor failed: %v", err)
		}

		if cursorResourceId == 0 {
			t.Fatal("expected non-zero cursor resource ID")
		}

		cursorRef := resClient.CreateResourceReference(cursorResourceId)
		defer cursorRef.Release()

		t.Logf("Successfully built storage cursor from transaction, resource_id: %d", cursorResourceId)
	})

	t.Run("BuildStorageCursorOnEngine", func(t *testing.T) {
		resClient, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		cursorResourceId, err := engine.BuildStorageCursor(ctx)
		if err != nil {
			t.Fatalf("BuildStorageCursor failed: %v", err)
		}

		if cursorResourceId == 0 {
			t.Fatal("expected non-zero cursor resource ID")
		}

		cursorRef := resClient.CreateResourceReference(cursorResourceId)
		defer cursorRef.Release()

		t.Logf("Successfully built storage cursor from engine, resource_id: %d", cursorResourceId)
	})

	t.Run("AccessWorldStateOnTx", func(t *testing.T) {
		resClient, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		tx, err := engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer tx.Release()

		cursorResourceId, err := tx.AccessWorldState(ctx, nil)
		if err != nil {
			t.Fatalf("AccessWorldState failed: %v", err)
		}

		if cursorResourceId == 0 {
			t.Fatal("expected non-zero cursor resource ID")
		}

		cursorRef := resClient.CreateResourceReference(cursorResourceId)
		defer cursorRef.Release()

		t.Logf("Successfully accessed world state from transaction, resource_id: %d", cursorResourceId)
	})

	t.Run("AccessWorldStateOnEngine", func(t *testing.T) {
		resClient, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		cursorResourceId, err := engine.AccessWorldState(ctx, nil)
		if err != nil {
			t.Fatalf("AccessWorldState failed: %v", err)
		}

		if cursorResourceId == 0 {
			t.Fatal("expected non-zero cursor resource ID")
		}

		cursorRef := resClient.CreateResourceReference(cursorResourceId)
		defer cursorRef.Release()

		t.Logf("Successfully accessed world state from engine, resource_id: %d", cursorResourceId)
	})
}

// TestWatchWorldState tests the reactive WorldState watch functionality.
func TestWatchWorldState(t *testing.T) {
	ctx := context.Background()

	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()

	t.Run("ReceivesInitialResourceId", func(t *testing.T) {
		_, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		stream, err := engine.WatchWorldState(ctx)
		if err != nil {
			t.Fatalf("WatchWorldState failed: %v", err)
		}

		msg, err := stream.Recv()
		if err != nil {
			t.Fatalf("Recv failed: %v", err)
		}

		if msg.ResourceId == 0 {
			t.Fatal("expected non-zero resource_id")
		}

		t.Logf("Received initial resource_id: %d", msg.ResourceId)
	})

	t.Run("DetectsObjectChanges", func(t *testing.T) {
		resClient, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		stream, err := engine.WatchWorldState(ctx)
		if err != nil {
			t.Fatalf("WatchWorldState failed: %v", err)
		}

		initialMsg, err := stream.Recv()
		if err != nil {
			t.Fatalf("Recv failed: %v", err)
		}
		t.Logf("Initial resource_id: %d", initialMsg.ResourceId)

		// Get tracked WorldState to access
		trackedRef := resClient.CreateResourceReference(initialMsg.ResourceId)
		defer trackedRef.Release()

		trackedWs, err := s4wave_world.NewWorldState(resClient, trackedRef, false)
		if err != nil {
			t.Fatalf("NewWorldState failed: %v", err)
		}

		// Access an object through tracked WorldState to register tracking
		objKey := "test-watch-object-" + t.Name()
		_, found, err := trackedWs.GetObject(ctx, objKey)
		if err != nil {
			t.Fatalf("GetObject failed: %v", err)
		}
		if found {
			t.Fatal("expected object not found initially")
		}

		// Create a NEW write transaction for making changes
		writeTx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction for write failed: %v", err)
		}
		defer writeTx.Release()

		// Make changes through write transaction
		obj, err := writeTx.CreateObject(ctx, objKey, &bucket.ObjectRef{})
		if err != nil {
			t.Fatalf("CreateObject failed: %v", err)
		}

		_, err = obj.IncrementRev(ctx)
		if err != nil {
			t.Fatalf("IncrementRev failed: %v", err)
		}

		// Commit the write transaction
		err = writeTx.Commit(ctx)
		if err != nil {
			t.Fatalf("Commit failed: %v", err)
		}

		// Watch should detect changes
		changeMsg, err := stream.Recv()
		if err != nil {
			t.Fatalf("Recv after change failed: %v", err)
		}

		if changeMsg.ResourceId == initialMsg.ResourceId {
			t.Fatal("expected different resource_id after change")
		}

		t.Logf("Detected change - new resource_id: %d", changeMsg.ResourceId)
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		_, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		watchCtx, watchCancel := context.WithCancel(ctx)

		stream, err := engine.WatchWorldState(watchCtx)
		if err != nil {
			t.Fatalf("WatchWorldState failed: %v", err)
		}

		_, err = stream.Recv()
		if err != nil {
			t.Fatalf("Recv failed: %v", err)
		}

		watchCancel()

		_, err = stream.Recv()
		if err == nil {
			t.Fatal("expected error after context cancellation")
		}
		if err != io.EOF && err != context.Canceled {
			t.Logf("Got expected error: %v", err)
		}

		t.Log("Correctly handled context cancellation")
	})

	t.Run("UniqueResourceIds", func(t *testing.T) {
		resClient, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		stream, err := engine.WatchWorldState(ctx)
		if err != nil {
			t.Fatalf("WatchWorldState failed: %v", err)
		}

		msg1, err := stream.Recv()
		if err != nil {
			t.Fatalf("Recv failed: %v", err)
		}

		if msg1.ResourceId == 0 {
			t.Fatal("expected non-zero initial resource_id")
		}
		t.Logf("Received initial resource_id: %d", msg1.ResourceId)

		seenIds := map[uint32]bool{msg1.ResourceId: true}

		trackedRef := resClient.CreateResourceReference(msg1.ResourceId)
		defer trackedRef.Release()
		trackedWs, err := s4wave_world.NewWorldState(resClient, trackedRef, false)
		if err != nil {
			t.Fatalf("NewWorldState failed: %v", err)
		}

		for i := range 3 {
			objKey := fmt.Sprintf("test-unique-%s-%d", t.Name(), i)

			// Access object through tracked WorldState to register tracking
			_, _, err := trackedWs.GetObject(ctx, objKey)
			if err != nil {
				t.Fatalf("GetObject failed: %v", err)
			}

			// Create a NEW write transaction for each change
			writeTx, err := engine.NewTransaction(ctx, true)
			if err != nil {
				t.Fatalf("NewTransaction for write failed: %v", err)
			}

			obj, err := writeTx.CreateObject(ctx, objKey, &bucket.ObjectRef{})
			if err != nil {
				writeTx.Release()
				t.Fatalf("CreateObject failed: %v", err)
			}

			_, err = obj.IncrementRev(ctx)
			if err != nil {
				writeTx.Release()
				t.Fatalf("IncrementRev failed: %v", err)
			}

			// Commit the write transaction
			err = writeTx.Commit(ctx)
			if err != nil {
				writeTx.Release()
				t.Fatalf("Commit failed: %v", err)
			}
			writeTx.Release()

			msg, err := stream.Recv()
			if err != nil {
				t.Fatalf("Recv failed: %v", err)
			}

			if seenIds[msg.ResourceId] {
				t.Fatalf("duplicate resource_id: %d", msg.ResourceId)
			}
			seenIds[msg.ResourceId] = true
			t.Logf("Received unique resource_id %d after change %d", msg.ResourceId, i+1)

			trackedRef.Release()
			trackedRef = resClient.CreateResourceReference(msg.ResourceId)
			trackedWs, err = s4wave_world.NewWorldState(resClient, trackedRef, false)
			if err != nil {
				t.Fatalf("NewWorldState failed: %v", err)
			}
		}

		if len(seenIds) != 4 {
			t.Fatalf("expected 4 unique resource_ids, got %d", len(seenIds))
		}

		t.Log("Successfully received unique resource IDs for each change")
	})
}
