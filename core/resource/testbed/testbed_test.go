package resource_testbed_test

import (
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
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
	t.Helper()
	_, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
	t.Cleanup(cleanup)

	rootRef := resClient.AccessRootResource()
	t.Cleanup(rootRef.Release)

	srpcClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	return &rpcWorldFixture{
		resClient:     resClient,
		testbedClient: s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient),
	}
}

// createEngineRef creates a world via RPC and returns a tracked reference to
// the engine resource. Release is registered via t.Cleanup.
func (f *rpcWorldFixture) createEngineRef(ctx context.Context, t *testing.T) resource_client.ResourceRef {
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
	ctx := context.Background()

	// Test 1: Create testbed resource server and access root
	t.Run("AccessRootResource", func(t *testing.T) {
		f := setupRPCWorldFixture(ctx, t)

		// Sanity-check the root resource ID via the underlying ref API.
		rootRef := f.resClient.AccessRootResource()
		defer rootRef.Release()

		rootID := rootRef.GetResourceID()
		if rootID == 0 {
			t.Fatal("expected non-zero root resource ID")
		}

		t.Logf("Successfully accessed root resource with ID: %d", rootID)
	})

	// Test 2: Create world engine via CreateWorld RPC
	t.Run("CreateWorld", func(t *testing.T) {
		f := setupRPCWorldFixture(ctx, t)

		resp, err := f.testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
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
		f := setupRPCWorldFixture(ctx, t)
		engineRef := f.createEngineRef(ctx, t)

		ec, _ := engineClient(t, engineRef)
		infoResp, err := ec.GetEngineInfo(ctx, &s4wave_world.GetEngineInfoRequest{})
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
		f := setupRPCWorldFixture(ctx, t)
		engineRef := f.createEngineRef(ctx, t)

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

		if waitResp.Seqno < newSeqno {
			t.Fatalf("expected seqno >= %d, got %d", newSeqno, waitResp.Seqno)
		}

		t.Logf("Successfully waited for seqno %d", waitResp.Seqno)
	})

	// Test 5: Multiple engine resources can be created
	t.Run("MultipleEngines", func(t *testing.T) {
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

		if resp1.ResourceId == resp2.ResourceId {
			t.Fatal("expected different resource IDs for different engines")
		}

		t.Logf("Created two engines with IDs: %d, %d", resp1.ResourceId, resp2.ResourceId)
	})

	// Test 6: WatchWorldState via transaction
	t.Run("WatchWorldStateViaTransaction", func(t *testing.T) {
		f := setupRPCWorldFixture(ctx, t)
		engineRef := f.createEngineRef(ctx, t)

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

		if msg.ResourceId == 0 {
			t.Fatal("expected non-zero resource_id from WatchWorldState")
		}

		t.Logf("Received initial resource_id from watch: %d", msg.ResourceId)
	})

	// Test 7: Resource cleanup on release
	t.Run("ResourceCleanup", func(t *testing.T) {
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
	t.Helper()
	f := setupRPCWorldFixture(ctx, t)
	createResp, err := f.testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
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
	ctx := context.Background()

	// Test 1: Create world engine and get engine info
	t.Run("CreateWorldAndGetInfo", func(t *testing.T) {
		f := setupSDKEngine(ctx, t)

		infoResp, err := f.engine.GetEngineInfo(ctx)
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
		f := setupSDKEngine(ctx, t)

		initialSeqno, err := f.engine.GetSeqno(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Logf("Initial seqno: %d", initialSeqno)

		tx, err := f.engine.NewTransaction(ctx, true)
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

		newSeqno, err := f.engine.GetSeqno(ctx)
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
		f := setupSDKEngine(ctx, t)

		tx, err := f.engine.NewTransaction(ctx, true)
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
		f := setupSDKEngine(ctx, t)

		tx, err := f.engine.NewTransaction(ctx, true)
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
		f := setupSDKEngine(ctx, t)

		tx, err := f.engine.NewTransaction(ctx, true)
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

		newSeqno, err := f.engine.GetSeqno(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		waitedSeqno, err := f.engine.WaitSeqno(ctx, newSeqno)
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
		f := setupSDKEngine(ctx, t)

		stream, err := f.engine.WatchWorldState(ctx)
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
		// Manual engine lifecycle: this test releases the engine in the body,
		// so we do not use the t.Cleanup-based setupSDKEngine helper.
		f := setupRPCWorldFixture(ctx, t)
		createResp, err := f.testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
		if err != nil {
			t.Fatal(err.Error())
		}
		engineRef := f.resClient.CreateResourceReference(createResp.ResourceId)
		engine, err := s4wave_world.NewEngine(f.resClient, engineRef)
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

// stateAtomFixture wraps an accessed StateAtom resource client for a subtest.
type stateAtomFixture struct {
	stateClient resource_state.SRPCStateAtomResourceServiceClient
}

// setupStateAtom creates a testbed and accesses a StateAtom on the requested
// store ID (empty string defaults to the default store).
func setupStateAtom(ctx context.Context, t *testing.T, storeID string) *stateAtomFixture {
	t.Helper()
	f := setupRPCWorldFixture(ctx, t)
	accessResp, err := f.testbedClient.AccessStateAtom(ctx, &s4wave_testbed.AccessStateAtomRequest{
		StoreId: storeID,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
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
				getResp, err := sf.stateClient.GetState(ctx, &resource_state.GetStateRequest{})
				if err != nil {
					t.Fatal(err.Error())
				}

				if getResp.StateJson != "{}" {
					t.Fatalf("expected initial state '{}', got %q", getResp.StateJson)
				}

				t.Logf("Initial state: %s, seqno: %d", getResp.StateJson, getResp.Seqno)
			},
		},
		{
			name: "SetAndGetState",
			run: func(t *testing.T, sf *stateAtomFixture) {
				testState := `{"tabs":[{"id":"home","path":"/"}],"activeTabId":"home"}`
				setResp, err := sf.stateClient.SetState(ctx, &resource_state.SetStateRequest{
					StateJson: testState,
				})
				if err != nil {
					t.Fatal(err.Error())
				}

				if setResp.Seqno == 0 {
					t.Fatal("expected non-zero seqno after SetState")
				}

				getResp, err := sf.stateClient.GetState(ctx, &resource_state.GetStateRequest{})
				if err != nil {
					t.Fatal(err.Error())
				}

				if getResp.StateJson != testState {
					t.Fatalf("expected state %q, got %q", testState, getResp.StateJson)
				}

				t.Logf("Set and retrieved state successfully, seqno: %d", getResp.Seqno)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sf := setupStateAtom(ctx, t, "")
			tc.run(t, sf)
		})
	}

	// Test 4: WatchState receives updates - non-trivial case kept as its own run.
	t.Run("WatchStateUpdates", func(t *testing.T) {
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

		if msg.Seqno <= initialSeqno {
			t.Fatalf("expected seqno > %d, got %d", initialSeqno, msg.Seqno)
		}

		if msg.StateJson != `{"updated":true}` {
			t.Fatalf("expected updated state, got %q", msg.StateJson)
		}

		t.Logf("Received updated state: %s, seqno: %d", msg.StateJson, msg.Seqno)
	})

	// Test 5: Custom store ID - non-trivial case kept as its own run.
	// Both stores must live in the same testbed/resClient for isolation to be
	// observable, so this case shares the rpcWorldFixture rather than calling
	// setupStateAtom twice.
	t.Run("CustomStoreId", func(t *testing.T) {
		f := setupRPCWorldFixture(ctx, t)

		customResp, err := f.testbedClient.AccessStateAtom(ctx, &s4wave_testbed.AccessStateAtomRequest{
			StoreId: "custom-store",
		})
		if err != nil {
			t.Fatal(err.Error())
		}
		customRef := f.resClient.CreateResourceReference(customResp.ResourceId)
		defer customRef.Release()
		customSrpc, err := customRef.GetClient()
		if err != nil {
			t.Fatal(err.Error())
		}
		customClient := resource_state.NewSRPCStateAtomResourceServiceClient(customSrpc)
		_, err = customClient.SetState(ctx, &resource_state.SetStateRequest{
			StateJson: `{"custom":true}`,
		})
		if err != nil {
			t.Fatal(err.Error())
		}

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

		if getResp.StateJson != "{}" {
			t.Fatalf("expected default store to have '{}', got %q", getResp.StateJson)
		}

		t.Log("Custom and default stores are properly isolated")
	})
}
