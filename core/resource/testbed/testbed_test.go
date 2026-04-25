package resource_testbed_test

import (
	"context"
	"io"
	"testing"

	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// TestTestbedResourceServerViaRpc tests the testbed resource server functionality calling RPCs directly.
func TestTestbedResourceServerViaRpc(t *testing.T) {
	ctx := context.Background()

	// Test 1: Create testbed resource server and access root
	t.Run("AccessRootResource", func(t *testing.T) {
		_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
		defer cleanup()

		// Access root resource
		rootRef := resClient.AccessRootResource()
		defer rootRef.Release()

		rootID := rootRef.GetResourceID()
		if rootID == 0 {
			t.Fatal("expected non-zero root resource ID")
		}

		t.Logf("Successfully accessed root resource with ID: %d", rootID)
	})

	// Test 2: Create world engine via CreateWorld RPC
	t.Run("CreateWorld", func(t *testing.T) {
		_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
		defer cleanup()

		// Access root resource
		rootRef := resClient.AccessRootResource()
		defer rootRef.Release()

		// Call CreateWorld
		srpcClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
		resp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		if resp.ResourceId == 0 {
			t.Fatal("expected non-zero resource_id from CreateWorld")
		}

		t.Logf("Created world engine with resource_id: %d", resp.ResourceId)
	})

	// Test 3: Get engine info from created engine resource
	t.Run("GetEngineInfo", func(t *testing.T) {
		_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
		defer cleanup()

		// Access root and create world
		rootRef := resClient.AccessRootResource()
		defer rootRef.Release()

		srpcClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
		createResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Create reference to engine resource
		engineRef := resClient.CreateResourceReference(createResp.ResourceId)
		defer engineRef.Release()

		// Get engine info
		engineSrpcClient, err := engineRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		engineClient := s4wave_world.NewSRPCEngineResourceServiceClient(engineSrpcClient)
		infoResp, err := engineClient.GetEngineInfo(ctx, &s4wave_world.GetEngineInfoRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		if infoResp.GetEngineInfo().GetEngineId() == "" {
			t.Fatal("expected non-empty engine_id")
		}
		if infoResp.GetEngineInfo().GetBucketId() == "" {
			t.Fatal("expected non-empty bucket_id")
		}

		t.Logf("Engine info - ID: %s, Bucket: %s",
			infoResp.GetEngineInfo().GetEngineId(), infoResp.GetEngineInfo().GetBucketId())
	})

	// Test 4: Access WorldState operations via transaction
	t.Run("WorldStateOperations", func(t *testing.T) {
		_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
		defer cleanup()

		// Access root and create world
		rootRef := resClient.AccessRootResource()
		defer rootRef.Release()

		srpcClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
		createResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Create reference to engine resource
		engineRef := resClient.CreateResourceReference(createResp.ResourceId)
		defer engineRef.Release()

		// Create EngineResourceService client
		engineSrpcClient, err := engineRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		engineClient := s4wave_world.NewSRPCEngineResourceServiceClient(engineSrpcClient)

		// Test GetSeqno via engine
		seqnoResp, err := engineClient.GetSeqno(ctx, &s4wave_world.GetSeqnoRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Logf("Initial seqno: %d", seqnoResp.Seqno)

		// Create a write transaction
		txResp, err := engineClient.NewTransaction(ctx, &s4wave_world.NewTransactionRequest{
			Write: true,
		})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Create reference to transaction resource
		txRef := resClient.CreateResourceReference(txResp.ResourceId)
		defer txRef.Release()

		// Create WorldStateResourceService client on the transaction
		txSrpcClient, err := txRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		worldStateClient := s4wave_world.NewSRPCWorldStateResourceServiceClient(txSrpcClient)

		// Create an object via RPC
		objKey := "test-obj-" + t.Name()
		createObjResp, err := worldStateClient.CreateObject(ctx, &s4wave_world.CreateObjectRequest{
			ObjectKey: objKey,
		})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Create reference to object resource
		objRef := resClient.CreateResourceReference(createObjResp.ResourceId)
		defer objRef.Release()

		// Create ObjectStateResourceService client
		objSrpcClient, err := objRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		objStateClient := s4wave_world.NewSRPCObjectStateResourceServiceClient(objSrpcClient)

		// Increment revision via RPC
		_, err = objStateClient.IncrementRev(ctx, &s4wave_world.IncrementRevRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Commit the transaction
		txClient := s4wave_world.NewSRPCTxResourceServiceClient(txSrpcClient)
		_, err = txClient.Commit(ctx, &s4wave_world.CommitRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Get the new seqno after the commit
		newSeqnoResp, err := engineClient.GetSeqno(ctx, &s4wave_world.GetSeqnoRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}
		newSeqno := newSeqnoResp.Seqno

		// Wait for seqno via RPC (should return immediately since we already have the seqno)
		waitResp, err := engineClient.WaitSeqno(ctx, &s4wave_world.WaitSeqnoRequest{
			Seqno: newSeqno,
		})
		if err != nil {
			t.Fatal(err.Error())
		}

		if waitResp.Seqno < newSeqno {
			t.Fatalf("expected seqno >= %d, got %d", newSeqno, waitResp.Seqno)
		}

		t.Logf("Successfully waited for seqno %d", waitResp.Seqno)
	})

	// Test 5: Multiple engine resources can be created
	t.Run("MultipleEngines", func(t *testing.T) {
		_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
		defer cleanup()

		// Access root resource
		rootRef := resClient.AccessRootResource()
		defer rootRef.Release()

		srpcClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)

		// Create first engine
		resp1, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Create second engine
		resp2, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		if resp1.ResourceId == resp2.ResourceId {
			t.Fatal("expected different resource IDs for different engines")
		}

		t.Logf("Created two engines with IDs: %d, %d", resp1.ResourceId, resp2.ResourceId)
	})

	// Test 6: WatchWorldState via transaction
	t.Run("WatchWorldStateViaTransaction", func(t *testing.T) {
		_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
		defer cleanup()

		// Access root and create world
		rootRef := resClient.AccessRootResource()
		defer rootRef.Release()

		srpcClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
		createResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Create reference to engine resource
		engineRef := resClient.CreateResourceReference(createResp.ResourceId)
		defer engineRef.Release()

		// Create EngineResourceService client
		engineSrpcClient, err := engineRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		engineClient := s4wave_world.NewSRPCEngineResourceServiceClient(engineSrpcClient)

		// Create a read transaction
		txResp, err := engineClient.NewTransaction(ctx, &s4wave_world.NewTransactionRequest{
			Write: false,
		})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Create reference to transaction resource
		txRef := resClient.CreateResourceReference(txResp.ResourceId)
		defer txRef.Release()

		// Create WatchWorldStateResourceService client on the engine
		watchClient := s4wave_world.NewSRPCWatchWorldStateResourceServiceClient(engineSrpcClient)

		// Start watching
		stream, err := watchClient.WatchWorldState(ctx, &s4wave_world.WatchWorldStateRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Should receive initial resource_id
		msg, err := stream.Recv()
		if err != nil {
			t.Fatal(err.Error())
		}

		if msg.ResourceId == 0 {
			t.Fatal("expected non-zero resource_id from WatchWorldState")
		}

		t.Logf("Received initial resource_id from watch: %d", msg.ResourceId)
	})

	// Test 7: Resource cleanup on release
	t.Run("ResourceCleanup", func(t *testing.T) {
		_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
		defer cleanup()

		// Access root and create world
		rootRef := resClient.AccessRootResource()
		defer rootRef.Release()

		srpcClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
		createResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Create reference to engine resource
		engineRef := resClient.CreateResourceReference(createResp.ResourceId)

		// Create WatchWorldStateResourceService client on the engine
		engineSrpcClient, err := engineRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		watchClient := s4wave_world.NewSRPCWatchWorldStateResourceServiceClient(engineSrpcClient)

		// Start a watch to verify it's working
		stream, err := watchClient.WatchWorldState(ctx, &s4wave_world.WatchWorldStateRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Receive initial message with tracked WorldState resource
		_, err = stream.Recv()
		if err != nil {
			t.Fatal(err.Error())
		}

		// Release the engine reference
		engineRef.Release()

		// Next Recv should fail because engine resource is cleaned up
		_, err = stream.Recv()
		if err == nil {
			t.Fatal("expected error after releasing engine resource")
		}
		if err != io.EOF {
			t.Logf("Got expected error after release: %v", err)
		}

		t.Log("Resource successfully cleaned up after release")
	})
}

// TestTestbedResourceServerViaSDK tests the testbed resource server functionality using SDK wrappers.
func TestTestbedResourceServerViaSDK(t *testing.T) {
	ctx := context.Background()

	// Test 1: Create world engine and get engine info
	t.Run("CreateWorldAndGetInfo", func(t *testing.T) {
		_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
		defer cleanup()

		rootRef := resClient.AccessRootResource()
		defer rootRef.Release()

		srpcClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
		createResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		engineRef := resClient.CreateResourceReference(createResp.ResourceId)
		engine, err := s4wave_world.NewEngine(resClient, engineRef)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer engine.Release()

		infoResp, err := engine.GetEngineInfo(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		info := infoResp.GetEngineInfo()
		if info.GetEngineId() == "" {
			t.Fatal("expected non-empty engine_id")
		}
		if info.GetBucketId() == "" {
			t.Fatal("expected non-empty bucket_id")
		}

		t.Logf("Engine info - ID: %s, Bucket: %s", info.GetEngineId(), info.GetBucketId())
	})

	// Test 2: Create and commit transaction
	t.Run("CreateAndCommitTransaction", func(t *testing.T) {
		_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
		defer cleanup()

		rootRef := resClient.AccessRootResource()
		defer rootRef.Release()

		srpcClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
		createResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		engineRef := resClient.CreateResourceReference(createResp.ResourceId)
		engine, err := s4wave_world.NewEngine(resClient, engineRef)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer engine.Release()

		initialSeqno, err := engine.GetSeqno(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Logf("Initial seqno: %d", initialSeqno)

		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer tx.Release()

		objKey := "test-obj-" + t.Name()
		obj, err := tx.CreateObject(ctx, objKey, nil)
		if err != nil {
			t.Fatal(err.Error())
		}

		_, err = obj.IncrementRev(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		err = tx.Commit(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		newSeqno, err := engine.GetSeqno(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		if newSeqno <= initialSeqno {
			t.Fatalf("expected seqno to increase, got %d <= %d", newSeqno, initialSeqno)
		}

		t.Logf("Seqno increased from %d to %d after commit", initialSeqno, newSeqno)
	})

	// Test 3: WorldState operations
	t.Run("WorldStateOperations", func(t *testing.T) {
		_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
		defer cleanup()

		rootRef := resClient.AccessRootResource()
		defer rootRef.Release()

		srpcClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
		createResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		engineRef := resClient.CreateResourceReference(createResp.ResourceId)
		engine, err := s4wave_world.NewEngine(resClient, engineRef)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer engine.Release()

		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer tx.Release()

		objKey := "test-ws-obj-" + t.Name()

		_, found, err := tx.GetObject(ctx, objKey)
		if err != nil {
			t.Fatal(err.Error())
		}
		if found {
			t.Fatal("expected object not found initially")
		}

		obj, err := tx.CreateObject(ctx, objKey, nil)
		if err != nil {
			t.Fatal(err.Error())
		}

		key := obj.GetKey()
		if key != objKey {
			t.Fatalf("expected key %q, got %q", objKey, key)
		}

		retrievedObj, found, err := tx.GetObject(ctx, objKey)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !found {
			t.Fatal("expected object found after create")
		}

		retrievedKey := retrievedObj.GetKey()
		if retrievedKey != objKey {
			t.Fatalf("expected retrieved key %q, got %q", objKey, retrievedKey)
		}

		deleted, err := tx.DeleteObject(ctx, objKey)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !deleted {
			t.Fatal("expected deleted=true")
		}

		_, found, err = tx.GetObject(ctx, objKey)
		if err != nil {
			t.Fatal(err.Error())
		}
		if found {
			t.Fatal("expected object not found after delete")
		}

		t.Log("Successfully performed WorldState operations")
	})

	// Test 4: ObjectState operations
	t.Run("ObjectStateOperations", func(t *testing.T) {
		_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
		defer cleanup()

		rootRef := resClient.AccessRootResource()
		defer rootRef.Release()

		srpcClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
		createResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		engineRef := resClient.CreateResourceReference(createResp.ResourceId)
		engine, err := s4wave_world.NewEngine(resClient, engineRef)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer engine.Release()

		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer tx.Release()

		objKey := "test-objstate-" + t.Name()
		obj, err := tx.CreateObject(ctx, objKey, nil)
		if err != nil {
			t.Fatal(err.Error())
		}

		_, initialRev, err := obj.GetRootRef(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		if initialRev != 1 {
			t.Fatalf("expected initial rev=1, got %d", initialRev)
		}

		newRev, err := obj.IncrementRev(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		if newRev != 2 {
			t.Fatalf("expected rev=2 after increment, got %d", newRev)
		}

		_, afterIncRev, err := obj.GetRootRef(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		if afterIncRev != 2 {
			t.Fatalf("expected GetRootRef rev=2, got %d", afterIncRev)
		}

		t.Log("Successfully performed ObjectState operations")
	})

	// Test 5: WaitSeqno
	t.Run("WaitSeqno", func(t *testing.T) {
		_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
		defer cleanup()

		rootRef := resClient.AccessRootResource()
		defer rootRef.Release()

		srpcClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
		createResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		engineRef := resClient.CreateResourceReference(createResp.ResourceId)
		engine, err := s4wave_world.NewEngine(resClient, engineRef)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer engine.Release()

		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer tx.Release()

		objKey := "test-wait-" + t.Name()
		obj, err := tx.CreateObject(ctx, objKey, nil)
		if err != nil {
			t.Fatal(err.Error())
		}

		_, err = obj.IncrementRev(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		err = tx.Commit(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		newSeqno, err := engine.GetSeqno(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		waitedSeqno, err := engine.WaitSeqno(ctx, newSeqno)
		if err != nil {
			t.Fatal(err.Error())
		}

		if waitedSeqno < newSeqno {
			t.Fatalf("expected waited seqno >= %d, got %d", newSeqno, waitedSeqno)
		}

		t.Logf("Successfully waited for seqno %d", waitedSeqno)
	})

	// Test 6: WatchWorldState
	t.Run("WatchWorldState", func(t *testing.T) {
		_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
		defer cleanup()

		rootRef := resClient.AccessRootResource()
		defer rootRef.Release()

		srpcClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
		createResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		engineRef := resClient.CreateResourceReference(createResp.ResourceId)
		engine, err := s4wave_world.NewEngine(resClient, engineRef)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer engine.Release()

		stream, err := engine.WatchWorldState(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		msg, err := stream.Recv()
		if err != nil {
			t.Fatal(err.Error())
		}

		if msg.ResourceId == 0 {
			t.Fatal("expected non-zero resource_id from WatchWorldState")
		}

		t.Logf("Received initial resource_id from watch: %d", msg.ResourceId)
	})

	// Test 7: Resource cleanup
	t.Run("ResourceCleanupViaSDK", func(t *testing.T) {
		_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
		defer cleanup()

		rootRef := resClient.AccessRootResource()
		defer rootRef.Release()

		srpcClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
		createResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		engineRef := resClient.CreateResourceReference(createResp.ResourceId)
		engine, err := s4wave_world.NewEngine(resClient, engineRef)
		if err != nil {
			t.Fatal(err.Error())
		}

		stream, err := engine.WatchWorldState(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		_, err = stream.Recv()
		if err != nil {
			t.Fatal(err.Error())
		}

		engine.Release()

		_, err = stream.Recv()
		if err == nil {
			t.Fatal("expected error after releasing engine resource")
		}
		if err != io.EOF {
			t.Logf("Got expected error after release: %v", err)
		}

		t.Log("Resource successfully cleaned up after release")
	})
}

// TestStateAtomResourceViaRpc tests the StateAtom resource functionality via raw RPC calls.
func TestStateAtomResourceViaRpc(t *testing.T) {
	ctx := context.Background()

	// Test 1: Access StateAtom resource
	t.Run("AccessStateAtom", func(t *testing.T) {
		_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
		defer cleanup()

		rootRef := resClient.AccessRootResource()
		defer rootRef.Release()

		srpcClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)

		// Access StateAtom (uses default store ID)
		resp, err := testbedClient.AccessStateAtom(ctx, &s4wave_testbed.AccessStateAtomRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		if resp.ResourceId == 0 {
			t.Fatal("expected non-zero resource_id from AccessStateAtom")
		}

		t.Logf("Accessed StateAtom resource with ID: %d", resp.ResourceId)
	})

	// Test 2: Get initial state (should be empty JSON object)
	t.Run("GetInitialState", func(t *testing.T) {
		_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
		defer cleanup()

		rootRef := resClient.AccessRootResource()
		defer rootRef.Release()

		srpcClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)

		// Access StateAtom
		accessResp, err := testbedClient.AccessStateAtom(ctx, &s4wave_testbed.AccessStateAtomRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Create reference to state atom resource
		stateRef := resClient.CreateResourceReference(accessResp.ResourceId)
		defer stateRef.Release()

		// Get state
		stateSrpcClient, err := stateRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		stateClient := resource_state.NewSRPCStateAtomResourceServiceClient(stateSrpcClient)

		getResp, err := stateClient.GetState(ctx, &resource_state.GetStateRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		if getResp.StateJson != "{}" {
			t.Fatalf("expected initial state '{}', got %q", getResp.StateJson)
		}

		t.Logf("Initial state: %s, seqno: %d", getResp.StateJson, getResp.Seqno)
	})

	// Test 3: Set and get state
	t.Run("SetAndGetState", func(t *testing.T) {
		_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
		defer cleanup()

		rootRef := resClient.AccessRootResource()
		defer rootRef.Release()

		srpcClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)

		// Access StateAtom
		accessResp, err := testbedClient.AccessStateAtom(ctx, &s4wave_testbed.AccessStateAtomRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		stateRef := resClient.CreateResourceReference(accessResp.ResourceId)
		defer stateRef.Release()

		stateSrpcClient, err := stateRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		stateClient := resource_state.NewSRPCStateAtomResourceServiceClient(stateSrpcClient)

		// Set state
		testState := `{"tabs":[{"id":"home","path":"/"}],"activeTabId":"home"}`
		setResp, err := stateClient.SetState(ctx, &resource_state.SetStateRequest{
			StateJson: testState,
		})
		if err != nil {
			t.Fatal(err.Error())
		}

		if setResp.Seqno == 0 {
			t.Fatal("expected non-zero seqno after SetState")
		}

		// Get state back
		getResp, err := stateClient.GetState(ctx, &resource_state.GetStateRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		if getResp.StateJson != testState {
			t.Fatalf("expected state %q, got %q", testState, getResp.StateJson)
		}

		t.Logf("Set and retrieved state successfully, seqno: %d", getResp.Seqno)
	})

	// Test 4: WatchState receives updates
	t.Run("WatchStateUpdates", func(t *testing.T) {
		_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
		defer cleanup()

		rootRef := resClient.AccessRootResource()
		defer rootRef.Release()

		srpcClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)

		// Access StateAtom
		accessResp, err := testbedClient.AccessStateAtom(ctx, &s4wave_testbed.AccessStateAtomRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		stateRef := resClient.CreateResourceReference(accessResp.ResourceId)
		defer stateRef.Release()

		stateSrpcClient, err := stateRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		stateClient := resource_state.NewSRPCStateAtomResourceServiceClient(stateSrpcClient)

		// Start watching
		stream, err := stateClient.WatchState(ctx, &resource_state.WatchStateRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Should receive initial state
		msg, err := stream.Recv()
		if err != nil {
			t.Fatal(err.Error())
		}

		initialSeqno := msg.Seqno
		t.Logf("Received initial state: %s, seqno: %d", msg.StateJson, msg.Seqno)

		// Set state in a goroutine
		done := make(chan struct{})
		go func() {
			defer close(done)
			testState := `{"updated":true}`
			_, err := stateClient.SetState(ctx, &resource_state.SetStateRequest{
				StateJson: testState,
			})
			if err != nil {
				t.Errorf("SetState failed: %v", err)
			}
		}()

		// Wait for the set to complete
		<-done

		// Should receive updated state
		msg, err = stream.Recv()
		if err != nil {
			t.Fatal(err.Error())
		}

		if msg.Seqno <= initialSeqno {
			t.Fatalf("expected seqno > %d, got %d", initialSeqno, msg.Seqno)
		}

		if msg.StateJson != `{"updated":true}` {
			t.Fatalf("expected updated state, got %q", msg.StateJson)
		}

		t.Logf("Received updated state: %s, seqno: %d", msg.StateJson, msg.Seqno)
	})

	// Test 5: Custom store ID
	t.Run("CustomStoreId", func(t *testing.T) {
		_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
		defer cleanup()

		rootRef := resClient.AccessRootResource()
		defer rootRef.Release()

		srpcClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)

		// Access StateAtom with custom store ID
		accessResp, err := testbedClient.AccessStateAtom(ctx, &s4wave_testbed.AccessStateAtomRequest{
			StoreId: "custom-store",
		})
		if err != nil {
			t.Fatal(err.Error())
		}

		stateRef := resClient.CreateResourceReference(accessResp.ResourceId)
		defer stateRef.Release()

		stateSrpcClient, err := stateRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		stateClient := resource_state.NewSRPCStateAtomResourceServiceClient(stateSrpcClient)

		// Set state on custom store
		_, err = stateClient.SetState(ctx, &resource_state.SetStateRequest{
			StateJson: `{"custom":true}`,
		})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Access default store and verify it's separate
		defaultResp, err := testbedClient.AccessStateAtom(ctx, &s4wave_testbed.AccessStateAtomRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		defaultRef := resClient.CreateResourceReference(defaultResp.ResourceId)
		defer defaultRef.Release()

		defaultSrpcClient, err := defaultRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		defaultClient := resource_state.NewSRPCStateAtomResourceServiceClient(defaultSrpcClient)

		// Default store should still have empty state
		getResp, err := defaultClient.GetState(ctx, &resource_state.GetStateRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		if getResp.StateJson != "{}" {
			t.Fatalf("expected default store to have '{}', got %q", getResp.StateJson)
		}

		t.Log("Custom and default stores are properly isolated")
	})
}
