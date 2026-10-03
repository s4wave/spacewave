//go:build !js

package resource_testbed_test

import (
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	"github.com/s4wave/spacewave/db/world"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// rpcWorldFixture holds the RPC plumbing shared by RPC-variant subtests.
type rpcWorldFixture struct {
	resClient     *resource_client.Client
	testbedClient s4wave_testbed.SRPCTestbedResourceServiceClient
}

// setupRPCWorldFixture creates a testbed, resource client, root reference, and
// testbed RPC client. Cleanup is registered via t.Cleanup.
func setupRPCWorldFixture(ctx context.Context, t *testing.T) *rpcWorldFixture {
	// Create the World testbed and register its cleanup.
	t.Helper()
	_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
	t.Cleanup(cleanup)

	// Hold the testbed root resource for this fixture.
	rootRef := resClient.AccessRootResource()
	t.Cleanup(rootRef.Release)

	// Connect the fixture to the root testbed RPC service.
	srpcClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Return the resource connection and its testbed RPC client.
	return &rpcWorldFixture{
		resClient:     resClient,
		testbedClient: s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient),
	}
}

// createEngineRef creates a world via RPC and returns a tracked reference to
// the engine resource. Release is registered via t.Cleanup.
func (f *rpcWorldFixture) createEngineRef(ctx context.Context, t *testing.T) resource_client.ResourceRef {
	// Create an engine resource and register its reference cleanup.
	t.Helper()
	createResp, err := f.testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	engineRef := f.resClient.CreateResourceReference(createResp.ResourceId)
	t.Cleanup(engineRef.Release)
	return engineRef
}

// engineClient builds an EngineResourceService client from an engine ref and
// returns the underlying SRPC client for callers that also need it.
func engineClient(t *testing.T, engineRef resource_client.ResourceRef) (s4wave_world.SRPCEngineResourceServiceClient, srpc.Client) {
	t.Helper()
	engineSrpcClient, err := engineRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	return s4wave_world.NewSRPCEngineResourceServiceClient(engineSrpcClient), engineSrpcClient
}

// TestTestbedResourceServerViaRpc tests the testbed resource server functionality calling RPCs directly.
func TestTestbedResourceServerViaRpc(t *testing.T) {
	// Share a context across the testbed RPC scenarios.
	ctx := context.Background()

	// Test 1: Create testbed resource server and access root
	t.Run("AccessRootResource", func(t *testing.T) {
		// Create a fresh RPC fixture for the root resource check.
		f := setupRPCWorldFixture(ctx, t)

		// Sanity-check the root resource ID via the underlying ref API.
		rootRef := f.resClient.AccessRootResource()
		defer rootRef.Release()

		// Require a usable root resource identifier.
		rootID := rootRef.GetResourceID()
		if rootID == 0 {
			t.Fatal("expected non-zero root resource ID")
		}

		// Report the root resource identifier returned by the client.
		t.Logf("Successfully accessed root resource with ID: %d", rootID)
	})

	// Test 2: Create world engine via CreateWorld RPC
	t.Run("CreateWorld", func(t *testing.T) {
		// Create a fresh RPC fixture for World creation.
		f := setupRPCWorldFixture(ctx, t)

		// Create a World through the testbed RPC service.
		resp, err := f.testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Require a resource identifier for the created World.
		if resp.ResourceId == 0 {
			t.Fatal("expected non-zero resource_id from CreateWorld")
		}

		// Report the created World resource identifier.
		t.Logf("Created world engine with resource_id: %d", resp.ResourceId)
	})

	// Test 3: Get engine info from created engine resource
	t.Run("GetEngineInfo", func(t *testing.T) {
		// Create a World engine reference for its metadata check.
		f := setupRPCWorldFixture(ctx, t)
		engineRef := f.createEngineRef(ctx, t)
		defer engineRef.Release()

		// Read the created World engine metadata over RPC.
		ec, _ := engineClient(t, engineRef)
		infoResp, err := ec.GetEngineInfo(ctx, &s4wave_world.GetEngineInfoRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Require both engine and bucket identifiers in the metadata.
		if infoResp.GetEngineInfo().GetEngineId() == "" {
			t.Fatal("expected non-empty engine_id")
		}
		if infoResp.GetEngineInfo().GetBucketId() == "" {
			t.Fatal("expected non-empty bucket_id")
		}

		// Report the engine identifiers returned over RPC.
		t.Logf("Engine info - ID: %s, Bucket: %s",
			infoResp.GetEngineInfo().GetEngineId(), infoResp.GetEngineInfo().GetBucketId())
	})

	// Test 4: Access WorldState operations via transaction
	t.Run("WorldStateOperations", func(t *testing.T) {
		// Create a World engine for transaction operations.
		f := setupRPCWorldFixture(ctx, t)
		engineRef := f.createEngineRef(ctx, t)
		defer engineRef.Release()

		// Connect to the World engine RPC service.
		ec, _ := engineClient(t, engineRef)

		// Test GetSeqno via engine
		seqnoResp, err := ec.GetSeqno(ctx, &s4wave_world.GetSeqnoRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Logf("Initial seqno: %d", seqnoResp.Seqno)

		// Create a write transaction
		txResp, err := ec.NewTransaction(ctx, &s4wave_world.NewTransactionRequest{
			Write: true,
		})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Create reference to transaction resource
		txRef := f.resClient.CreateResourceReference(txResp.ResourceId)
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
		objRef := f.resClient.CreateResourceReference(createObjResp.ResourceId)
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
		newSeqnoResp, err := ec.GetSeqno(ctx, &s4wave_world.GetSeqnoRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}
		newSeqno := newSeqnoResp.Seqno

		// Wait for seqno via RPC (should return immediately since we already have the seqno)
		waitResp, err := ec.WaitSeqno(ctx, &s4wave_world.WaitSeqnoRequest{
			Seqno: newSeqno,
		})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Require WaitSeqno to reach the committed World sequence.
		if waitResp.Seqno < newSeqno {
			t.Fatalf("expected seqno >= %d, got %d", newSeqno, waitResp.Seqno)
		}

		// Report the World sequence reached by WaitSeqno.
		t.Logf("Successfully waited for seqno %d", waitResp.Seqno)
	})

	// Test 5: Multiple engine resources can be created
	t.Run("MultipleEngines", func(t *testing.T) {
		// Create a fresh RPC fixture for independent World engines.
		f := setupRPCWorldFixture(ctx, t)

		// Create first engine
		resp1, err := f.testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Create second engine
		resp2, err := f.testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Require distinct resource identifiers for the two World engines.
		if resp1.ResourceId == resp2.ResourceId {
			t.Fatal("expected different resource IDs for different engines")
		}

		// Report the resource identifiers of both World engines.
		t.Logf("Created two engines with IDs: %d, %d", resp1.ResourceId, resp2.ResourceId)
	})

	// Test 6: WatchWorldState via transaction
	t.Run("WatchWorldStateViaTransaction", func(t *testing.T) {
		// Create a World engine for the state watch.
		f := setupRPCWorldFixture(ctx, t)
		engineRef := f.createEngineRef(ctx, t)
		defer engineRef.Release()

		// Connect the engine RPC clients for the transaction and watch.
		ec, engineSrpcClient := engineClient(t, engineRef)

		// Create a read transaction
		txResp, err := ec.NewTransaction(ctx, &s4wave_world.NewTransactionRequest{
			Write: false,
		})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Create reference to transaction resource
		txRef := f.resClient.CreateResourceReference(txResp.ResourceId)
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

		// Require a resource identifier in the initial World state event.
		if msg.ResourceId == 0 {
			t.Fatal("expected non-zero resource_id from WatchWorldState")
		}

		// Report the initial resource identifier from the World watch.
		t.Logf("Received initial resource_id from watch: %d", msg.ResourceId)
	})

	// Test 7: Resource cleanup on release
	t.Run("ResourceCleanup", func(t *testing.T) {
		// Create a fresh RPC fixture for explicit engine cleanup.
		f := setupRPCWorldFixture(ctx, t)

		// Manual lifecycle: this test releases the engine ref explicitly
		// during the body, so we do not register Release with t.Cleanup.
		createResp, err := f.testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}
		engineRef := f.resClient.CreateResourceReference(createResp.ResourceId)

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

		// Report successful engine resource cleanup over RPC.
		t.Log("Resource successfully cleaned up after release")
	})
}

// sdkEngineFixture wraps an SDK engine bound to a freshly created world.
type sdkEngineFixture struct {
	engine *s4wave_world.Engine
}

// setupSDKEngine creates a testbed, resource client, and an SDK Engine over a
// freshly created world. Cleanup is registered via t.Cleanup.
func setupSDKEngine(ctx context.Context, t *testing.T) *sdkEngineFixture {
	// Create the RPC fixture for an SDK World engine.
	t.Helper()
	f := setupRPCWorldFixture(ctx, t)
	createResp, err := f.testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Wrap the created engine resource and register SDK cleanup.
	engineRef := f.resClient.CreateResourceReference(createResp.ResourceId)
	engine, err := s4wave_world.NewEngine(f.resClient, engineRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(engine.Release)
	return &sdkEngineFixture{engine: engine}
}

// TestTestbedResourceServerViaSDK tests the testbed resource server functionality using SDK wrappers.
func TestTestbedResourceServerViaSDK(t *testing.T) {
	// Share a context across the SDK World scenarios.
	ctx := context.Background()

	// Test 1: Create world engine and get engine info
	t.Run("CreateWorldAndGetInfo", func(t *testing.T) {
		// Create an SDK engine for the metadata check.
		f := setupSDKEngine(ctx, t)

		// Read the World engine metadata through the SDK.
		infoResp, err := f.engine.GetEngineInfo(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Require engine and bucket identifiers in the SDK metadata.
		info := infoResp.GetEngineInfo()
		if info.GetEngineId() == "" {
			t.Fatal("expected non-empty engine_id")
		}
		if info.GetBucketId() == "" {
			t.Fatal("expected non-empty bucket_id")
		}

		// Report the SDK engine metadata identifiers.
		t.Logf("Engine info - ID: %s, Bucket: %s", info.GetEngineId(), info.GetBucketId())
	})

	// Test 2: Create and commit transaction
	t.Run("CreateAndCommitTransaction", func(t *testing.T) {
		// Create an SDK engine for the commit check.
		f := setupSDKEngine(ctx, t)

		// Record the World sequence before the SDK transaction.
		initialSeqno, err := f.engine.GetSeqno(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Logf("Initial seqno: %d", initialSeqno)

		// Open a write transaction on the SDK engine.
		tx, err := f.engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer tx.Release()

		// Create a World object in the SDK transaction.
		objKey := "test-obj-" + t.Name()
		obj, err := tx.CreateObject(ctx, objKey, nil)
		defer world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Advance the new object revision before committing.
		_, err = obj.IncrementRev(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Commit the SDK write transaction.
		err = tx.Commit(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Read the World sequence after the commit.
		newSeqno, err := f.engine.GetSeqno(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Require the committed World sequence to advance.
		if newSeqno <= initialSeqno {
			t.Fatalf("expected seqno to increase, got %d <= %d", newSeqno, initialSeqno)
		}

		// Report the World sequence change caused by the SDK commit.
		t.Logf("Seqno increased from %d to %d after commit", initialSeqno, newSeqno)
	})

	// Test 3: WorldState operations
	t.Run("WorldStateOperations", func(t *testing.T) {
		// Create an SDK engine for World object operations.
		f := setupSDKEngine(ctx, t)

		// Open a write transaction for the object checks.
		tx, err := f.engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer tx.Release()

		// Choose the World object key used throughout the transaction.
		objKey := "test-ws-obj-" + t.Name()

		// Require the World object to be absent before creation.
		objectState, found, err := tx.GetObject(ctx, objKey)
		world.ReleaseObjectState(objectState)
		if err != nil {
			t.Fatal(err.Error())
		}
		if found {
			t.Fatal("expected object not found initially")
		}

		// Create the World object in the transaction.
		obj, err := tx.CreateObject(ctx, objKey, nil)
		defer world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Require the created object to retain its requested key.
		key := obj.GetKey()
		if key != objKey {
			t.Fatalf("expected key %q, got %q", objKey, key)
		}

		// Retrieve the World object after creation.
		retrievedObj, found, err := tx.GetObject(ctx, objKey)
		defer world.ReleaseObjectState(retrievedObj)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !found {
			t.Fatal("expected object found after create")
		}

		// Require the retrieved object to retain the requested key.
		retrievedKey := retrievedObj.GetKey()
		if retrievedKey != objKey {
			t.Fatalf("expected retrieved key %q, got %q", objKey, retrievedKey)
		}

		// Delete the World object from the transaction.
		deleted, err := tx.DeleteObject(ctx, objKey)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !deleted {
			t.Fatal("expected deleted=true")
		}

		// Require the World object to be absent after deletion.
		var objectState2 world.ObjectState
		objectState2, found, err = tx.GetObject(ctx, objKey)
		world.ReleaseObjectState(objectState2)
		if err != nil {
			t.Fatal(err.Error())
		}
		if found {
			t.Fatal("expected object not found after delete")
		}

		// Report successful World object operations through the SDK.
		t.Log("Successfully performed WorldState operations")
	})

	// Test 4: ObjectState operations
	t.Run("ObjectStateOperations", func(t *testing.T) {
		// Create an SDK engine for object revision checks.
		f := setupSDKEngine(ctx, t)

		// Open a write transaction for object revision changes.
		tx, err := f.engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer tx.Release()

		// Create the World object whose revision will advance.
		objKey := "test-objstate-" + t.Name()
		obj, err := tx.CreateObject(ctx, objKey, nil)
		defer world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Require the new object to begin at revision one.
		_, initialRev, err := obj.GetRootRef(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		if initialRev != 1 {
			t.Fatalf("expected initial rev=1, got %d", initialRev)
		}

		// Advance the World object revision through the SDK.
		newRev, err := obj.IncrementRev(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Require the increment result to be revision two.
		if newRev != 2 {
			t.Fatalf("expected rev=2 after increment, got %d", newRev)
		}

		// Require the stored object root to report revision two.
		_, afterIncRev, err := obj.GetRootRef(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		if afterIncRev != 2 {
			t.Fatalf("expected GetRootRef rev=2, got %d", afterIncRev)
		}

		// Report successful World object revision operations.
		t.Log("Successfully performed ObjectState operations")
	})

	// Test 5: WaitSeqno
	t.Run("WaitSeqno", func(t *testing.T) {
		// Create an SDK engine for the sequence wait check.
		f := setupSDKEngine(ctx, t)

		// Open a write transaction to advance the World sequence.
		tx, err := f.engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer tx.Release()

		// Create a World object for the sequence change.
		objKey := "test-wait-" + t.Name()
		obj, err := tx.CreateObject(ctx, objKey, nil)
		defer world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Advance the World object revision in the transaction.
		_, err = obj.IncrementRev(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Commit the World object change.
		err = tx.Commit(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Read the committed World sequence from the engine.
		newSeqno, err := f.engine.GetSeqno(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Wait for the SDK engine to reach the committed sequence.
		waitedSeqno, err := f.engine.WaitSeqno(ctx, newSeqno)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Require the sequence wait to reach the committed value.
		if waitedSeqno < newSeqno {
			t.Fatalf("expected waited seqno >= %d, got %d", newSeqno, waitedSeqno)
		}

		// Report the World sequence reached through the SDK wait.
		t.Logf("Successfully waited for seqno %d", waitedSeqno)
	})

	// Test 6: WatchWorldState
	t.Run("WatchWorldState", func(t *testing.T) {
		// Create an SDK engine for the World state watch.
		f := setupSDKEngine(ctx, t)

		// Subscribe to the SDK engine World state stream.
		stream, err := f.engine.WatchWorldState(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Receive the initial World state event.
		msg, err := stream.Recv()
		if err != nil {
			t.Fatal(err.Error())
		}

		// Require the initial event to identify a World state resource.
		if msg.ResourceId == 0 {
			t.Fatal("expected non-zero resource_id from WatchWorldState")
		}

		// Report the World state resource from the SDK watch.
		t.Logf("Received initial resource_id from watch: %d", msg.ResourceId)
	})

	// Test 7: Resource cleanup
	t.Run("ResourceCleanupViaSDK", func(t *testing.T) {
		// Manual engine lifecycle: this test releases the engine in the body,
		// so we do not use the t.Cleanup-based setupSDKEngine helper.
		f := setupRPCWorldFixture(ctx, t)
		createResp, err := f.testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Wrap the created resource as an SDK engine for explicit release.
		engineRef := f.resClient.CreateResourceReference(createResp.ResourceId)
		engine, err := s4wave_world.NewEngine(f.resClient, engineRef)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Subscribe to World state before releasing the engine.
		stream, err := engine.WatchWorldState(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Receive the initial World state event before cleanup.
		_, err = stream.Recv()
		if err != nil {
			t.Fatal(err.Error())
		}

		// Release the SDK engine while its watch is open.
		engine.Release()

		// Require the World watch to terminate after engine release.
		_, err = stream.Recv()
		if err == nil {
			t.Fatal("expected error after releasing engine resource")
		}
		if err != io.EOF {
			t.Logf("Got expected error after release: %v", err)
		}

		// Report successful SDK engine resource cleanup.
		t.Log("Resource successfully cleaned up after release")
	})
}

// stateAtomFixture wraps an accessed StateAtom resource client for a subtest.
type stateAtomFixture struct {
	stateClient resource_state.SRPCStateAtomResourceServiceClient
}

// setupStateAtom creates a testbed and accesses a StateAtom on the requested
// store ID (empty string defaults to the default store).
func setupStateAtom(ctx context.Context, t *testing.T, storeID string) *stateAtomFixture {
	// Create an RPC fixture for the requested state atom store.
	t.Helper()
	f := setupRPCWorldFixture(ctx, t)
	accessResp, err := f.testbedClient.AccessStateAtom(ctx, &s4wave_testbed.AccessStateAtomRequest{
		StoreId: storeID,
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Hold the state atom resource and connect its RPC client.
	stateRef := f.resClient.CreateResourceReference(accessResp.ResourceId)
	t.Cleanup(stateRef.Release)
	stateSrpcClient, err := stateRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	return &stateAtomFixture{
		stateClient: resource_state.NewSRPCStateAtomResourceServiceClient(stateSrpcClient),
	}
}

// TestStateAtomResourceViaRpc tests the StateAtom resource functionality via raw RPC calls.
func TestStateAtomResourceViaRpc(t *testing.T) {
	// Share a context across the state atom RPC scenarios.
	ctx := context.Background()

	// Trivial single-RPC checks: shape state via RPC, assert observable result.
	type stateAtomCase struct {
		name string
		run  func(t *testing.T, sf *stateAtomFixture)
	}
	cases := []stateAtomCase{
		{
			name: "AccessStateAtom",
			run: func(t *testing.T, sf *stateAtomFixture) {
				// AccessStateAtom is exercised inside setupStateAtom; verify
				// the resulting client can issue a GetState call.
				resp, err := sf.stateClient.GetState(ctx, &resource_state.GetStateRequest{})
				if err != nil {
					t.Fatal(err.Error())
				}
				t.Logf("Accessed StateAtom; initial state: %s", resp.StateJson)
			},
		},
		{
			name: "GetInitialState",
			run: func(t *testing.T, sf *stateAtomFixture) {
				// Read the initial state atom record.
				getResp, err := sf.stateClient.GetState(ctx, &resource_state.GetStateRequest{})
				if err != nil {
					t.Fatal(err.Error())
				}

				// Require the initial state atom to contain an empty object.
				if getResp.StateJson != "{}" {
					t.Fatalf("expected initial state '{}', got %q", getResp.StateJson)
				}

				// Report the initial state atom contents and sequence.
				t.Logf("Initial state: %s, seqno: %d", getResp.StateJson, getResp.Seqno)
			},
		},
		{
			name: "SetAndGetState",
			run: func(t *testing.T, sf *stateAtomFixture) {
				// Write a tab layout to the state atom.
				testState := `{"tabs":[{"id":"home","path":"/"}],"activeTabId":"home"}`
				setResp, err := sf.stateClient.SetState(ctx, &resource_state.SetStateRequest{
					StateJson: testState,
				})
				if err != nil {
					t.Fatal(err.Error())
				}

				// Require the state atom write to advance its sequence.
				if setResp.Seqno == 0 {
					t.Fatal("expected non-zero seqno after SetState")
				}

				// Read the state atom after the tab layout write.
				getResp, err := sf.stateClient.GetState(ctx, &resource_state.GetStateRequest{})
				if err != nil {
					t.Fatal(err.Error())
				}

				// Require the state atom to retain the written layout.
				if getResp.StateJson != testState {
					t.Fatalf("expected state %q, got %q", testState, getResp.StateJson)
				}

				// Report the state atom sequence after the layout round trip.
				t.Logf("Set and retrieved state successfully, seqno: %d", getResp.Seqno)
			},
		},
	}

	// Run each state atom case against a fresh default store.
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sf := setupStateAtom(ctx, t, "")
			tc.run(t, sf)
		})
	}

	// Test 4: WatchState receives updates - non-trivial case kept as its own run.
	t.Run("WatchStateUpdates", func(t *testing.T) {
		// Create a state atom fixture for streaming updates.
		sf := setupStateAtom(ctx, t, "")

		// Start watching
		stream, err := sf.stateClient.WatchState(ctx, &resource_state.WatchStateRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Should receive initial state
		msg, err := stream.Recv()
		if err != nil {
			t.Fatal(err.Error())
		}

		// Record the initial state atom sequence for comparison.
		initialSeqno := msg.Seqno
		t.Logf("Received initial state: %s, seqno: %d", msg.StateJson, msg.Seqno)

		// Set state in a goroutine
		done := make(chan struct{})
		go func() {
			defer close(done)
			testState := `{"updated":true}`
			_, err := sf.stateClient.SetState(ctx, &resource_state.SetStateRequest{
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

		// Require the state atom update to advance the sequence.
		if msg.Seqno <= initialSeqno {
			t.Fatalf("expected seqno > %d, got %d", initialSeqno, msg.Seqno)
		}

		// Require the state atom event to carry the updated contents.
		if msg.StateJson != `{"updated":true}` {
			t.Fatalf("expected updated state, got %q", msg.StateJson)
		}

		// Report the updated state atom contents and sequence.
		t.Logf("Received updated state: %s, seqno: %d", msg.StateJson, msg.Seqno)
	})

	// Test 5: Custom store ID - non-trivial case kept as its own run.
	// Both stores must live in the same testbed/resClient for isolation to be
	// observable, so this case shares the rpcWorldFixture rather than calling
	// setupStateAtom twice.
	t.Run("CustomStoreId", func(t *testing.T) {
		// Create one RPC fixture to compare custom and default stores.
		f := setupRPCWorldFixture(ctx, t)

		// Open the custom state atom store and retain its resource.
		customResp, err := f.testbedClient.AccessStateAtom(ctx, &s4wave_testbed.AccessStateAtomRequest{
			StoreId: "custom-store",
		})
		if err != nil {
			t.Fatal(err.Error())
		}
		customRef := f.resClient.CreateResourceReference(customResp.ResourceId)
		defer customRef.Release()

		// Connect to the custom state atom RPC service.
		customSrpc, err := customRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		customClient := resource_state.NewSRPCStateAtomResourceServiceClient(customSrpc)

		// Write distinct contents to the custom state atom store.
		_, err = customClient.SetState(ctx, &resource_state.SetStateRequest{
			StateJson: `{"custom":true}`,
		})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Open the default state atom store for the isolation check.
		defaultResp, err := f.testbedClient.AccessStateAtom(ctx, &s4wave_testbed.AccessStateAtomRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}
		defaultRef := f.resClient.CreateResourceReference(defaultResp.ResourceId)
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

		// Require the default store to retain its empty state.
		if getResp.StateJson != "{}" {
			t.Fatalf("expected default store to have '{}', got %q", getResp.StateJson)
		}

		// Report successful isolation of the two state atom stores.
		t.Log("Custom and default stores are properly isolated")
	})
}
