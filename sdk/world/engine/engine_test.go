//go:build !js

package sdk_world_engine_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_parent "github.com/s4wave/spacewave/db/world/parent"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// setupSDKEngine creates a testbed, resource client, and SDKEngine for testing.
func setupSDKEngine(ctx context.Context, t *testing.T) (*sdk_world_engine.SDKEngine, func()) {
	t.Helper()
	return setupSDKEngineWithResourceClient(ctx, t, func(client *resource_client.Client) sdk_world_engine.ResourceClient {
		return client
	})
}

func setupSDKEngineWithResourceClient(
	ctx context.Context,
	t *testing.T,
	wrapClient func(*resource_client.Client) sdk_world_engine.ResourceClient,
) (*sdk_world_engine.SDKEngine, func()) {
	// Build the resource testbed and prepare its root client.
	t.Helper()
	_, resClient, tbCleanup := resource_testbed.SetupTestbedWithClient(ctx, t)

	// Acquire the testbed's root RPC client.
	rootRef := resClient.AccessRootResource()
	srpcClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		tbCleanup()
		t.Fatal(err.Error())
	}

	// Create the testbed resource client.
	testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
	createResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
	if err != nil {
		rootRef.Release()
		tbCleanup()
		t.Fatal(err.Error())
	}

	// Create the SDK engine and retain its resource reference.
	engineRef := resClient.CreateResourceReference(createResp.ResourceId)
	engine, err := sdk_world_engine.NewSDKEngine(wrapClient(resClient), engineRef)
	if err != nil {
		engineRef.Release()
		rootRef.Release()
		tbCleanup()
		t.Fatal(err.Error())
	}

	// Release the SDK engine and both resource scopes during cleanup.
	cleanup := func() {
		engine.Release()
		rootRef.Release()
		tbCleanup()
	}

	return engine, cleanup
}

// TestSDKEngine_NewTransaction checks that each transaction reports the mode it
// was opened with.
func TestSDKEngine_NewTransaction(t *testing.T) {
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	for _, write := range []bool{true, false} {
		tx, err := engine.NewTransaction(ctx, write)
		if err != nil {
			t.Fatal(err.Error())
		}
		readOnly := tx.GetReadOnly()
		tx.Discard()
		if readOnly == write {
			t.Fatalf("NewTransaction(write=%v) reported read-only %v", write, readOnly)
		}
	}
}

func TestSDKEngine_DiscardReleasesWriteTransaction(t *testing.T) {
	// Start an engine whose client drops transaction references on release.
	ctx := context.Background()
	engine, cleanup := setupSDKEngineWithResourceClient(ctx, t, func(client *resource_client.Client) sdk_world_engine.ResourceClient {
		return releaseDroppingClient{client: client}
	})
	defer cleanup()

	// Discard a write transaction to release its server-side scope.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	tx.Discard()

	// Attempt a second write after the first transaction is discarded.
	secondDone := make(chan error, 1)
	go func() {
		// Bound the follow-up writer so a leaked scope fails promptly.
		secondCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		second, err := engine.NewTransaction(secondCtx, true)
		if err == nil {
			second.Discard()
		}
		secondDone <- err
	}()

	// Require the second writer to finish after the discard.
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("second write transaction failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("discarded write transaction still blocked the next writer")
	}
}

type releaseDroppingClient struct {
	client sdk_world_engine.ResourceClient
}

func (c releaseDroppingClient) CreateResourceReference(resourceID uint32) resource_client.ResourceRef {
	return releaseDroppingRef{ResourceRef: c.client.CreateResourceReference(resourceID)}
}

type releaseDroppingRef struct {
	resource_client.ResourceRef
}

func (r releaseDroppingRef) Release() {}

// TestSDKEngine_GetSeqno tests reading the sequence number.
func TestSDKEngine_GetSeqno(t *testing.T) {
	// Start an SDK engine for the sequence-number read.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Read the initial sequence number from the engine.
	seqno, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Record the sequence number returned by the resource.
	t.Logf("initial seqno: %d", seqno)
}

func TestSDKEngine_WorldRootSnapshots(t *testing.T) {
	// Start an SDK engine for root snapshot reads and watches.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Read the current World root snapshot.
	initial, err := engine.GetWorldRootSnapshot(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if initial.GetRootRef().GetBucketId() == "" {
		t.Fatalf("expected root bucket id, got %#v", initial.GetRootRef())
	}

	// Subscribe to subsequent World root snapshots.
	stream, err := engine.WatchWorldRootSnapshots(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Receive and compare the watch's initial snapshot.
	watched, err := stream.Recv()
	if err != nil {
		t.Fatal(err.Error())
	}
	if watched.GetSeqno() != initial.GetSeqno() || !watched.GetRootRef().EqualsRef(initial.GetRootRef()) {
		t.Fatalf("initial snapshot mismatch: get=%#v watch=%#v", initial, watched)
	}
}

func TestSDKTxCommitMutations(t *testing.T) {
	// Start an SDK engine for batched transaction mutations.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Apply dependent mutations in one transaction RPC.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	sdkTx, ok := tx.(*sdk_world_engine.SDKTx)
	if !ok {
		t.Fatalf("transaction type = %T, want *SDKTx", tx)
	}
	results, err := sdkTx.CommitMutations(ctx, []*s4wave_world.TransactionMutation{
		{Mutation: &s4wave_world.TransactionMutation_CreateObject{CreateObject: &s4wave_world.CreateObjectRequest{ObjectKey: "batch/a"}}},
		{Mutation: &s4wave_world.TransactionMutation_CreateObject{CreateObject: &s4wave_world.CreateObjectRequest{ObjectKey: "batch/b"}}},
		{Mutation: &s4wave_world.TransactionMutation_SetObjectRoot{SetObjectRoot: &s4wave_world.SetObjectRootMutation{ObjectKey: "batch/a"}}},
		{Mutation: &s4wave_world.TransactionMutation_SetGraphQuad{SetGraphQuad: &s4wave_world.SetGraphQuadRequest{Quad: &quad.Quad{Subject: "<batch/a>", Predicate: "<batch-rel>", Obj: "<batch/b>"}}}},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(results) != 4 || results[0].GetCreateObject().GetObjectKey() != "batch/a" || results[0].GetCreateObject().GetRev() == 0 || results[2].GetSetObjectRoot() == nil || results[3].GetSetGraphQuad() == nil {
		t.Fatalf("unexpected mutation results: %#v", results)
	}

	// Read from a new transaction only after the batch response succeeds.
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer readTx.Discard()
	{
		objectState, found, err := readTx.GetObject(ctx, "batch/a")
		world.ReleaseObjectState(objectState)
		if err != nil || !found {
			t.Fatalf("created object found=%v err=%v", found, err)
		}
	}
	quads, err := readTx.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("batch/a", "<batch-rel>", "batch/b", ""), 1)
	if err != nil || len(quads) != 1 {
		t.Fatalf("committed graph quads = %#v, err=%v", quads, err)
	}

	// A failed mutation discards the whole transaction and its successful prefix.
	failedTx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	failedSDKTx := failedTx.(*sdk_world_engine.SDKTx)
	_, err = failedSDKTx.CommitMutations(ctx, []*s4wave_world.TransactionMutation{
		{Mutation: &s4wave_world.TransactionMutation_CreateObject{CreateObject: &s4wave_world.CreateObjectRequest{ObjectKey: "batch/discarded"}}},
		{Mutation: &s4wave_world.TransactionMutation_SetObjectRoot{SetObjectRoot: &s4wave_world.SetObjectRootMutation{ObjectKey: "batch/missing"}}},
	})
	if err == nil {
		t.Fatal("expected failed mutation batch")
	}
	if err := failedSDKTx.Commit(ctx); err == nil {
		t.Fatal("expected failed transaction to be unusable")
	}

	// Verify the failed mutation batch did not publish its successful prefix.
	checkTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	{
		objectState2, found, err := checkTx.GetObject(ctx, "batch/discarded")
		world.ReleaseObjectState(objectState2)
		if err != nil || found {
			checkTx.Discard()
			t.Fatalf("discarded object found=%v err=%v", found, err)
		}
	}
	checkTx.Discard()

	// A canceled request releases the transaction before it can publish state.
	canceledTx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	canceledSDKTx := canceledTx.(*sdk_world_engine.SDKTx)

	// Cancel the mutation request before submitting its object creation.
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = canceledSDKTx.CommitMutations(canceledCtx, []*s4wave_world.TransactionMutation{
		{Mutation: &s4wave_world.TransactionMutation_CreateObject{CreateObject: &s4wave_world.CreateObjectRequest{ObjectKey: "batch/canceled"}}},
	})
	if err == nil {
		t.Fatal("expected canceled mutation batch")
	}
	if err := canceledSDKTx.Commit(ctx); err == nil {
		t.Fatal("expected canceled transaction to be unusable")
	}

	// Verify cancellation left no object in the World.
	canceledCheckTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer canceledCheckTx.Discard()
	{
		objectState3, found, err := canceledCheckTx.GetObject(ctx, "batch/canceled")
		world.ReleaseObjectState(objectState3)
		if err != nil || found {
			t.Fatalf("canceled object found=%v err=%v", found, err)
		}
	}
}

// TestSDKEngine_WaitSeqno tests waiting for a sequence number.
func TestSDKEngine_WaitSeqno(t *testing.T) {
	// Start an SDK engine for sequence-number waits.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Create and commit a write transaction to advance seqno.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	var createdObject world.ObjectState
	createdObject, err = tx.CreateObject(ctx, "wait-seqno-obj", nil)
	world.ReleaseObjectState(createdObject)
	if err != nil {
		tx.Discard()
		t.Fatal(err.Error())
	}
	err = tx.Commit(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Read the sequence number advanced by the committed object.
	seqno, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Wait for the already-published sequence number.
	waited, err := engine.WaitSeqno(ctx, seqno)
	if err != nil {
		t.Fatal(err.Error())
	}
	if waited < seqno {
		t.Fatalf("expected waited seqno >= %d, got %d", seqno, waited)
	}

	// Record the sequence number observed by the wait.
	t.Logf("waited for seqno %d", waited)
}

// TestSDKEngine_StorageCursorWrites tests that resource-backed engine cursors
// reject writes and staged cursors accept them.
func TestSDKEngine_StorageCursorWrites(t *testing.T) {
	// Open a resource-backed SDK engine.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// The engine cursor reads the World but rejects writes.
	cursor, err := engine.BuildStorageCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer cursor.Release()
	if _, _, err := cursor.PutBlock(ctx, []byte("cursor-data"), &block.PutOpts{}); err == nil {
		t.Fatal("expected the engine cursor to reject a write")
	}

	// Build a staged cursor.
	stage, err := engine.StageWorldState(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer stage.Release()
	stagedCursor, err := stage.BuildStorageCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer stagedCursor.Release()

	// Write a block and read it back.
	ref, _, err := stagedCursor.PutBlock(ctx, []byte("cursor-data"), &block.PutOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}
	data, found, err := stagedCursor.GetBlock(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found || string(data) != "cursor-data" {
		t.Fatalf("expected staged cursor-data, got %q found=%v", string(data), found)
	}
}

// TestSDKEngine_CreateAndGetObject tests object creation and retrieval.
func TestSDKEngine_CreateAndGetObject(t *testing.T) {
	// Start an SDK engine for object creation and retrieval.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Open a write transaction for the object lifecycle checks.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()

	// Name the object used by the create and lookup assertions.
	objKey := "test-obj-create-get"

	// Verify object does not exist yet.
	objectState, found, err := tx.GetObject(ctx, objKey)
	world.ReleaseObjectState(objectState)
	if err != nil {
		t.Fatal(err.Error())
	}
	if found {
		t.Fatal("expected object not found initially")
	}

	// Create the object.
	obj, err := tx.CreateObject(ctx, objKey, nil)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if obj.GetKey() != objKey {
		t.Fatalf("expected key %q, got %q", objKey, obj.GetKey())
	}

	// Verify object exists now.
	retrieved, found, err := tx.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(retrieved)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.Fatal("expected object found after create")
	}
	if retrieved.GetKey() != objKey {
		t.Fatalf("expected retrieved key %q, got %q", objKey, retrieved.GetKey())
	}
}

// TestSDKEngine_DeleteObject tests object deletion.
func TestSDKEngine_DeleteObject(t *testing.T) {
	// Start an SDK engine for the object deletion checks.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Open a write transaction for the object lifecycle checks.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()

	// Name the object to create, delete, and verify absent.
	objKey := "test-obj-delete"

	// Create the object that the transaction will delete.
	var createdObject world.ObjectState
	createdObject, err = tx.CreateObject(ctx, objKey, nil)
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Delete the object and require the transaction to report success.
	deleted, err := tx.DeleteObject(ctx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !deleted {
		t.Fatal("expected deleted=true")
	}

	// Verify the deleted object is absent from the transaction.
	objectState, found, err := tx.GetObject(ctx, objKey)
	world.ReleaseObjectState(objectState)
	if err != nil {
		t.Fatal(err.Error())
	}
	if found {
		t.Fatal("expected object not found after delete")
	}
}

// TestSDKEngine_ListObjectsWithType tests the world-state typed object listing RPC.
func TestSDKEngine_ListObjectsWithType(t *testing.T) {
	// Start an SDK engine for typed object listing.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Open a write transaction to create and type World objects.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the candidate objects for typed listing.
	for _, key := range []string{"typed/a", "typed/b", "typed/c"} {
		{
			createdObject, err := tx.CreateObject(ctx, key, nil)
			world.ReleaseObjectState(createdObject)
			if err != nil {
				tx.Discard()
				t.Fatal(err.Error())
			}
		}
	}

	// Create the shared type object used by the graph edges.
	typeObjKey := world_types.BuildTypeObjectKey("sdk/type")
	{
		createdObject2, err := tx.CreateObject(ctx, typeObjKey, nil)
		world.ReleaseObjectState(createdObject2)
		if err != nil {
			tx.Discard()
			t.Fatal(err.Error())
		}
	}
	if err := tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("typed/a", world_types.TypePred.String(), typeObjKey, "")); err != nil {
		tx.Discard()
		t.Fatal(err.Error())
	}
	if err := tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("typed/c", world_types.TypePred.String(), typeObjKey, "")); err != nil {
		tx.Discard()
		t.Fatal(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Open a read transaction after committing the type links.
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer readTx.Discard()

	// Use the SDK transaction's typed-listing API.
	sdkReadTx, ok := readTx.(*sdk_world_engine.SDKTx)
	if !ok {
		t.Fatal("expected SDKTx")
	}

	// List objects linked to the requested type.
	objKeys, err := sdkReadTx.ListObjectsWithType(ctx, "sdk/type")
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(objKeys) != 2 {
		t.Fatalf("expected 2 typed objects, got %d", len(objKeys))
	}
	if objKeys[0] != "typed/a" || objKeys[1] != "typed/c" {
		t.Fatalf("unexpected typed object keys: %v", objKeys)
	}

	// Cross-check typed listing through the generic World helper.
	genericKeys, err := world_types.ListObjectsWithType(ctx, readTx, "sdk/type")
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(genericKeys) != 2 {
		t.Fatalf("expected 2 generic typed objects, got %d", len(genericKeys))
	}
	if genericKeys[0] != "typed/a" || genericKeys[1] != "typed/c" {
		t.Fatalf("unexpected generic typed object keys: %v", genericKeys)
	}
}

func TestSDKEngine_CheckObjectTypeThroughEngineWorldState(t *testing.T) {
	// Start an SDK engine for object type validation.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Create a typed object through the writable EngineWorldState.
	ws := world.NewEngineWorldState(engine, true)
	{
		createdObject, err := ws.CreateObject(ctx, "typed/check", nil)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	if err := world_types.SetObjectType(ctx, ws, "typed/check", "sdk/type-check"); err != nil {
		t.Fatal(err.Error())
	}

	// Check the persisted type through a read-only EngineWorldState.
	readWS := world.NewEngineWorldState(engine, false)
	if err := world_types.CheckObjectType(ctx, readWS, "typed/check", "sdk/type-check"); err != nil {
		t.Fatal(err.Error())
	}
}

func TestSDKEngine_ListObjectsWithTypeThroughEngineWorldState(t *testing.T) {
	// Start an SDK engine for typed listing through WorldState.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Create and type objects through the writable WorldState.
	ws := world.NewEngineWorldState(engine, true)

	// Populate the two objects included in the type listing.
	for _, key := range []string{"typed/engine-a", "typed/engine-b"} {
		{
			createdObject, err := ws.CreateObject(ctx, key, nil)
			world.ReleaseObjectState(createdObject)
			if err != nil {
				t.Fatal(err.Error())
			}
		}
		if err := world_types.SetObjectType(ctx, ws, key, "sdk/type-list"); err != nil {
			t.Fatal(err.Error())
		}
	}

	// List the persisted objects through a read-only WorldState.
	readWS := world.NewEngineWorldState(engine, false)
	objKeys, err := world_types.ListObjectsWithType(ctx, readWS, "sdk/type-list")
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(objKeys) != 2 {
		t.Fatalf("expected 2 typed objects, got %d", len(objKeys))
	}
	if objKeys[0] != "typed/engine-a" || objKeys[1] != "typed/engine-b" {
		t.Fatalf("unexpected typed object keys: %v", objKeys)
	}
}

// TestSDKEngine_AccessCayleyGraphUnsupported verifies SDK-backed worlds never
// synthesize a local Cayley handle by fetching every remote graph quad.
func TestSDKEngine_AccessCayleyGraphUnsupported(t *testing.T) {
	// Prepare a zero-value SDK WorldState for the unsupported graph check.
	ctx := context.Background()

	// Require the zero-value WorldState to reject local graph access.
	empty := &sdk_world_engine.SDKWorldState{}
	var called bool
	err := empty.AccessCayleyGraph(ctx, false, func(ctx context.Context, h world.CayleyHandle) error {
		called = true
		return nil
	})
	if !errors.Is(err, sdk_world_engine.ErrRemoteCayleyGraphUnsupported) {
		t.Fatalf("expected ErrRemoteCayleyGraphUnsupported from zero-value state, got %v", err)
	}
	if called {
		t.Fatal("unexpected Cayley callback call from zero-value state")
	}

	// Create an SDK engine backed by the testbed World.
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Open a read transaction for the remote graph-access check.
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()

	// Require SDK transactions to reject local Cayley graph callbacks.
	called = false
	err = tx.AccessCayleyGraph(ctx, false, func(ctx context.Context, h world.CayleyHandle) error {
		called = true
		return nil
	})
	if !errors.Is(err, sdk_world_engine.ErrRemoteCayleyGraphUnsupported) {
		t.Fatalf("expected ErrRemoteCayleyGraphUnsupported, got %v", err)
	}
	if called {
		t.Fatal("unexpected Cayley callback call")
	}
}

// TestSDKEngine_LookupGraphQuadsBatch tests bounded batch graph lookup RPCs.
func TestSDKEngine_LookupGraphQuadsBatch(t *testing.T) {
	// Start an SDK engine for bounded graph-quad batch lookup.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Open a write transaction to create the batch lookup graph.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create subjects and object for the graph lookup cases.
	for _, key := range []string{"batch/subj-a", "batch/subj-b", "batch/obj"} {
		{
			createdObject, err := tx.CreateObject(ctx, key, nil)
			world.ReleaseObjectState(createdObject)
			if err != nil {
				tx.Discard()
				t.Fatal(err.Error())
			}
		}
	}
	if err := tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("batch/subj-a", "<batch-rel>", "batch/obj", "")); err != nil {
		tx.Discard()
		t.Fatal(err.Error())
	}
	if err := tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("batch/subj-b", "<batch-rel>", "batch/obj", "")); err != nil {
		tx.Discard()
		t.Fatal(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Open a read transaction after committing both graph edges.
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer readTx.Discard()

	// Use the SDK transaction's batch lookup API.
	sdkReadTx, ok := readTx.(*sdk_world_engine.SDKTx)
	if !ok {
		t.Fatal("expected SDKTx")
	}

	// Query graph quads by subject and object in one batch.
	results, err := sdkReadTx.LookupGraphQuadsBatch(ctx, []world.GraphQuad{
		world.NewGraphQuadWithKeys("batch/subj-a", "<batch-rel>", "", ""),
		world.NewGraphQuadWithKeys("", "<batch-rel>", "batch/obj", ""),
	}, 10)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 result sets, got %d", len(results))
	}
	if len(results[0]) != 1 {
		t.Fatalf("expected 1 subject result, got %d", len(results[0]))
	}
	if len(results[1]) != 2 {
		t.Fatalf("expected 2 object results, got %d", len(results[1]))
	}

	// Reject batch requests with invalid limits or predicates.
	if _, err := sdkReadTx.LookupGraphQuadsBatch(ctx, []world.GraphQuad{
		world.NewGraphQuadWithKeys("batch/subj-a", "<batch-rel>", "", ""),
	}, 0); err == nil {
		t.Fatal("expected zero limit to fail")
	}
	if _, err := sdkReadTx.LookupGraphQuadsBatch(ctx, []world.GraphQuad{
		world.NewGraphQuadWithKeys("batch/subj-a", "", "", ""),
	}, 10); err == nil {
		t.Fatal("expected missing predicate to fail")
	}
}

// TestSDKEngine_ListObjects tests paging object keys under a prefix.
func TestSDKEngine_ListObjects(t *testing.T) {
	// Start an SDK engine on a fresh testbed World.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Create five objects under the listed prefix and one outside it.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Write the objects and commit them.
	want := []string{"list/a", "list/b", "list/c", "list/d", "list/e"}
	for _, key := range append(slices.Clone(want), "other/x") {
		createdObject, err := tx.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			tx.Discard()
			t.Fatal(err.Error())
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Open a read transaction on the SDK engine.
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer readTx.Discard()
	sdkReadTx, ok := readTx.(*sdk_world_engine.SDKTx)
	if !ok {
		t.Fatal("expected SDKTx")
	}

	// Page through the prefix two keys at a time.
	var got []string
	var startAfter string
	for {
		objects, _, more, err := sdkReadTx.ListObjects(ctx, "list/", "", startAfter, 2)
		if err != nil {
			t.Fatal(err.Error())
		}
		if len(objects) > 2 {
			t.Fatalf("expected at most 2 objects per page, got %d", len(objects))
		}
		for _, obj := range objects {
			got = append(got, obj.ObjectKey)
		}
		if !more {
			break
		}
		startAfter = objects[len(objects)-1].ObjectKey
	}

	// Every key under the prefix arrives once and in order.
	if !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}

	// Reject a zero limit and a cursor outside the prefix.
	if _, _, _, err := sdkReadTx.ListObjects(ctx, "list/", "", "", 0); err == nil {
		t.Fatal("expected zero limit to fail")
	}
	if _, _, _, err := sdkReadTx.ListObjects(ctx, "list/", "", "other/x", 2); err == nil {
		t.Fatal("expected cursor outside the prefix to fail")
	}
}

// TestSDKEngine_ListObjectsDelimiter tests listing one path level at a time.
func TestSDKEngine_ListObjectsDelimiter(t *testing.T) {
	// Start an SDK engine on a fresh testbed World.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Create objects at two levels under the prefix and one outside it.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	keys := []string{
		"tree/a",
		"tree/a/x",
		"tree/a/y",
		"tree/b",
		"tree/c/z/deep",
		"tree/d",
		"tree/e/w",
		"treehouse",
	}
	for _, key := range keys {
		createdObject, err := tx.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			tx.Discard()
			t.Fatal(err.Error())
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Open a read transaction on the SDK engine.
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer readTx.Discard()
	sdkReadTx, ok := readTx.(*sdk_world_engine.SDKTx)
	if !ok {
		t.Fatal("expected SDKTx")
	}

	// Page through the first level two entries at a time.
	var gotObjects, gotPrefixes []string
	var startAfter string
	for {
		objects, prefixes, more, err := sdkReadTx.ListObjects(ctx, "tree/", "/", startAfter, 2)
		if err != nil {
			t.Fatal(err.Error())
		}
		if len(objects)+len(prefixes) > 2 {
			t.Fatalf("expected at most 2 entries per page, got %d", len(objects)+len(prefixes))
		}
		for _, obj := range objects {
			gotObjects = append(gotObjects, obj.ObjectKey)
			startAfter = max(startAfter, obj.ObjectKey)
		}
		for _, prefix := range prefixes {
			gotPrefixes = append(gotPrefixes, prefix)
			startAfter = max(startAfter, prefix)
		}
		if !more {
			break
		}
	}

	// Each first-level object and group arrives once, and nothing outside the
	// prefix leaks in after the last group.
	wantObjects := []string{"tree/a", "tree/b", "tree/d"}
	if !slices.Equal(gotObjects, wantObjects) {
		t.Fatalf("expected objects %v, got %v", wantObjects, gotObjects)
	}
	wantPrefixes := []string{"tree/a/", "tree/c/", "tree/e/"}
	if !slices.Equal(gotPrefixes, wantPrefixes) {
		t.Fatalf("expected prefixes %v, got %v", wantPrefixes, gotPrefixes)
	}
}

// TestSDKEngine_GetObjectMetadataBatch tests remote-safe metadata fanout.
func TestSDKEngine_GetObjectMetadataBatch(t *testing.T) {
	// Start an SDK engine for object metadata batch reads.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Open a write transaction to create linked object metadata.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the parent and child metadata objects.
	for _, key := range []string{"metadata/parent", "metadata/child"} {
		{
			createdObject, err := tx.CreateObject(ctx, key, nil)
			world.ReleaseObjectState(createdObject)
			if err != nil {
				tx.Discard()
				t.Fatal(err.Error())
			}
		}
	}
	typeObjKey := world_types.BuildTypeObjectKey("sdk/metadata")
	{
		createdObject2, err := tx.CreateObject(ctx, typeObjKey, nil)
		world.ReleaseObjectState(createdObject2)
		if err != nil {
			tx.Discard()
			t.Fatal(err.Error())
		}
	}
	if err := tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("metadata/child", world_types.TypePred.String(), typeObjKey, "")); err != nil {
		tx.Discard()
		t.Fatal(err.Error())
	}
	if err := world_parent.SetObjectParent(ctx, tx, "metadata/child", "metadata/parent", true); err != nil {
		tx.Discard()
		t.Fatal(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Open a read transaction after committing object metadata.
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer readTx.Discard()

	// Read metadata for the child and its parent in one request.
	metadata, err := world_types.GetObjectMetadataBatch(ctx, readTx, []string{"metadata/child", "metadata/parent"})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(metadata) != 2 {
		t.Fatalf("expected 2 metadata entries, got %d", len(metadata))
	}
	if metadata[0].ObjectKey != "metadata/child" {
		t.Fatalf("unexpected child metadata key %q", metadata[0].ObjectKey)
	}
	if metadata[0].TypeID != "sdk/metadata" {
		t.Fatalf("expected child type sdk/metadata, got %q", metadata[0].TypeID)
	}
	if metadata[0].ParentObjectKey != "metadata/parent" {
		t.Fatalf("expected child parent metadata/parent, got %q", metadata[0].ParentObjectKey)
	}
	if metadata[1].ObjectKey != "metadata/parent" {
		t.Fatalf("unexpected parent metadata key %q", metadata[1].ObjectKey)
	}
	if metadata[1].TypeID != "" || metadata[1].ParentObjectKey != "" {
		t.Fatalf("unexpected parent metadata: %+v", metadata[1])
	}

	// Confirm the child retains its declared type.
	typeID, err := world_types.GetObjectType(ctx, readTx, "metadata/child")
	if err != nil {
		t.Fatal(err.Error())
	}
	if typeID != "sdk/metadata" {
		t.Fatalf("expected GetObjectType sdk/metadata, got %q", typeID)
	}
}

func TestSDKEngine_GetObjectRootRefsBatch(t *testing.T) {
	// Start an SDK engine for batched object root references.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Open a write transaction to set distinct object roots.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the first object with its root reference.
	{
		createdObject, err := tx.CreateObject(ctx, "root-ref/alpha", &bucket.ObjectRef{BucketId: "alpha-bucket"})
		world.ReleaseObjectState(createdObject)
		if err != nil {
			tx.Discard()
			t.Fatal(err.Error())
		}
	}

	// Create the second object with its root reference.
	{
		createdObject2, err := tx.CreateObject(ctx, "root-ref/beta", &bucket.ObjectRef{BucketId: "beta-bucket"})
		world.ReleaseObjectState(createdObject2)
		if err != nil {
			tx.Discard()
			t.Fatal(err.Error())
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Open a read transaction after publishing both root references.
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer readTx.Discard()

	// Read existing, missing, and duplicate object roots in one batch.
	refs, err := world.GetObjectRootRefsBatch(ctx, readTx, []string{"root-ref/beta", "missing", "root-ref/alpha", "root-ref/alpha"})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(refs) != 4 {
		t.Fatalf("expected 4 root refs, got %d", len(refs))
	}
	checkRootRef := func(ref *world.ObjectRootRef, key string, exists bool, bucketID string) {
		if ref.ObjectKey != key || ref.Exists != exists {
			t.Fatalf("unexpected root ref for %s: %+v", key, ref)
		}
		if !exists {
			if ref.RootRef != nil || ref.Rev != 0 {
				t.Fatalf("expected missing root ref for %s to be empty: %+v", key, ref)
			}
			return
		}
		if ref.RootRef.GetBucketId() != bucketID || ref.Rev != 1 {
			t.Fatalf("unexpected root ref for %s: %+v", key, ref)
		}
	}
	checkRootRef(refs[0], "root-ref/beta", true, "beta-bucket")
	checkRootRef(refs[1], "missing", false, "")
	checkRootRef(refs[2], "root-ref/alpha", true, "alpha-bucket")
	checkRootRef(refs[3], "root-ref/alpha", true, "alpha-bucket")
}

// TestSDKEngine_QueryGraphPath tests bounded remote graph path traversal.
func TestSDKEngine_QueryGraphPath(t *testing.T) {
	// Start an SDK engine for bounded graph path queries.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Open a write transaction to construct the path graph.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create all vertices used by the path queries.
	for _, key := range []string{"path/a", "path/b", "path/c", "path/d"} {
		{
			createdObject, err := tx.CreateObject(ctx, key, nil)
			world.ReleaseObjectState(createdObject)
			if err != nil {
				tx.Discard()
				t.Fatal(err.Error())
			}
		}
	}

	// Link the vertices with branching and chained edges.
	for _, edge := range [][2]string{
		{"path/a", "path/b"},
		{"path/a", "path/d"},
		{"path/b", "path/c"},
	} {
		if err := tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(edge[0], "<path-rel>", edge[1], "")); err != nil {
			tx.Discard()
			t.Fatal(err.Error())
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Open a read transaction after committing the path graph.
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer readTx.Discard()

	// Collect endpoints after following two outgoing path steps.
	keys, err := world.CollectGraphPathWithKeys(ctx, readTx, &world.GraphPathQuery{
		StartKeys: []string{"path/a"},
		Steps: []world.GraphPathStep{
			{
				Direction: world.GraphPathDirectionOut,
				Predicate: "<path-rel>",
				Limit:     10,
			},
			{
				Direction: world.GraphPathDirectionOut,
				Predicate: "<path-rel>",
				Limit:     10,
			},
		},
		ResultLimit: 10,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(keys) != 1 || keys[0] != "path/c" {
		t.Fatalf("expected path/c, got %#v", keys)
	}

	// Query one incoming path and retain its traversed quad.
	result, err := readTx.QueryGraphPath(ctx, &world.GraphPathQuery{
		StartKeys: []string{"path/c"},
		Steps: []world.GraphPathStep{
			{
				Direction: world.GraphPathDirectionIn,
				Predicate: "<path-rel>",
				Limit:     10,
			},
		},
		ResultLimit:  10,
		IncludeQuads: true,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(result.ObjectKeys) != 1 || result.ObjectKeys[0] != "path/b" {
		t.Fatalf("expected reverse path/b, got %#v", result.ObjectKeys)
	}
	if len(result.Quads) != 1 {
		t.Fatalf("expected 1 traversed quad, got %d", len(result.Quads))
	}

	// Reject graph path requests with zero result or step limits.
	if _, err := readTx.QueryGraphPath(ctx, &world.GraphPathQuery{
		StartKeys:   []string{"path/a"},
		ResultLimit: 0,
	}); err == nil {
		t.Fatal("expected zero result limit to fail")
	}
	if _, err := readTx.QueryGraphPath(ctx, &world.GraphPathQuery{
		StartKeys: []string{"path/a"},
		Steps: []world.GraphPathStep{
			{
				Direction: world.GraphPathDirectionOut,
				Predicate: "<path-rel>",
			},
		},
		ResultLimit: 10,
	}); err == nil {
		t.Fatal("expected zero step limit to fail")
	}
}

// TestSDKEngine_TransactionCommit tests committing a transaction and verifying seqno advances.
func TestSDKEngine_TransactionCommit(t *testing.T) {
	// Start an SDK engine for transaction sequence advancement.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Read the sequence number before committing a mutation.
	initial, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open a write transaction to advance the World root.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the object whose commit advances the sequence number.
	var createdObject world.ObjectState
	createdObject, err = tx.CreateObject(ctx, "commit-obj", nil)
	world.ReleaseObjectState(createdObject)
	if err != nil {
		tx.Discard()
		t.Fatal(err.Error())
	}

	// Commit the object creation to publish the next sequence number.
	err = tx.Commit(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Read the sequence number after the commit.
	after, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Require the committed transaction to advance the sequence number.
	if after <= initial {
		t.Fatalf("expected seqno to advance after commit, got %d <= %d", after, initial)
	}

	// Record the sequence numbers around the commit.
	t.Logf("seqno advanced from %d to %d", initial, after)
}

// TestSDKEngine_ObjectState tests object state operations.
func TestSDKEngine_ObjectState(t *testing.T) {
	// Start an SDK engine for object revision operations.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Open a write transaction to create the object state.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()

	// Create the object whose root revision will be checked.
	obj, err := tx.CreateObject(ctx, "objstate-test", nil)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}

	// GetRootRef should return initial revision.
	_, rev, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if rev != 1 {
		t.Fatalf("expected initial rev=1, got %d", rev)
	}

	// IncrementRev should advance the revision.
	newRev, err := obj.IncrementRev(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if newRev != 2 {
		t.Fatalf("expected rev=2 after increment, got %d", newRev)
	}

	// GetRootRef should reflect the new revision.
	_, checkRev, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if checkRev != 2 {
		t.Fatalf("expected rev=2 from GetRootRef, got %d", checkRev)
	}
}

// TestSDKEngine_IterateObjects exercises the lazy iterator after the unary
// IterateObjects request has returned. The iterator resource, not that request,
// controls its lifetime.
func TestSDKEngine_IterateObjects(t *testing.T) {
	// Start an SDK engine for lazy object iteration.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Create iterator fixtures in a write transaction.
	writeTx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"iterator/a", "iterator/b", "other/c"} {
		{
			createdObject, err := writeTx.CreateObject(ctx, key, nil)
			world.ReleaseObjectState(createdObject)
			if err != nil {
				writeTx.Discard()
				t.Fatal(err)
			}
		}
	}
	if err := writeTx.Commit(ctx); err != nil {
		writeTx.Discard()
		t.Fatal(err)
	}
	writeTx.Discard()

	// Open a read transaction after publishing the iterator fixtures.
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()

	// Iterate objects under the requested prefix.
	iter := readTx.IterateObjects(ctx, "iterator/", false)
	defer iter.Close()
	var keys []string
	for iter.Next() {
		if !iter.Valid() {
			break
		}
		keys = append(keys, iter.Key())
	}

	// Require iteration to complete without a resource error.
	if err := iter.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(keys, []string{"iterator/a", "iterator/b"}) {
		t.Fatalf("unexpected iterator keys %v", keys)
	}

	// Seek the iterator to the second key and verify its position.
	seek := readTx.IterateObjects(ctx, "iterator/", false)
	defer seek.Close()
	if err := seek.Seek("iterator/b"); err != nil {
		t.Fatal(err)
	}
	if !seek.Valid() || seek.Key() != "iterator/b" {
		t.Fatalf("seek returned valid=%v key=%q", seek.Valid(), seek.Key())
	}
}

// TestSDKEngine_GraphQuadOperations tests graph quad set, lookup, and delete.
func TestSDKEngine_GraphQuadOperations(t *testing.T) {
	// Start an SDK engine for graph-quad mutations.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Open a write transaction for graph-quad operations.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()

	// Create two objects so graph quads reference valid IRIs.
	var createdObject world.ObjectState
	createdObject, err = tx.CreateObject(ctx, "graph-subj", nil)
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err.Error())
	}
	var createdObject2 world.ObjectState
	createdObject2, err = tx.CreateObject(ctx, "graph-obj", nil)
	world.ReleaseObjectState(createdObject2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Set a graph quad.
	q := world.NewGraphQuadWithKeys("graph-subj", "<relates-to>", "graph-obj", "")
	err = tx.SetGraphQuad(ctx, q)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Lookup the quad.
	filter := world.NewGraphQuadWithKeys("graph-subj", "", "", "")
	quads, err := tx.LookupGraphQuads(ctx, filter, 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(quads) == 0 {
		t.Fatal("expected at least one quad from lookup")
	}

	// Record the successful graph-quad lookup count.
	t.Logf("found %d quad(s)", len(quads))

	// Delete the quad.
	err = tx.DeleteGraphQuad(ctx, q)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Lookup should return empty now.
	quads, err = tx.LookupGraphQuads(ctx, filter, 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(quads) != 0 {
		t.Fatalf("expected 0 quads after delete, got %d", len(quads))
	}
}

// TestSDKEngine_DeleteGraphObject tests deleting all quads for an object.
//
// Note: hydra's DeleteGraphObject has a known bug where it returns early
// if the object only appears as Subject or only as Object (uses || instead
// of && on the early-return check). The test sets up quads in both
// directions to work around this.
func TestSDKEngine_DeleteGraphObject(t *testing.T) {
	// Start an SDK engine for deleting all graph links of one object.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Open a write transaction for graph object deletion.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()

	// Create both objects referenced by the test quads.
	var createdObject world.ObjectState
	createdObject, err = tx.CreateObject(ctx, "dgo-subj", nil)
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err.Error())
	}
	var createdObject2 world.ObjectState
	createdObject2, err = tx.CreateObject(ctx, "dgo-obj", nil)
	world.ReleaseObjectState(createdObject2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Set quads in both directions so dgo-subj appears as both Subject and Object.
	// This is required because hydra's DeleteGraphObject returns early if the
	// object only appears in one position (known bug: || instead of &&).
	q1 := world.NewGraphQuadWithKeys("dgo-subj", "<ref>", "dgo-obj", "")
	err = tx.SetGraphQuad(ctx, q1)
	if err != nil {
		t.Fatal(err.Error())
	}
	q2 := world.NewGraphQuadWithKeys("dgo-obj", "<back-ref>", "dgo-subj", "")
	err = tx.SetGraphQuad(ctx, q2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Delete every graph quad that references the selected object.
	err = tx.DeleteGraphObject(ctx, "dgo-subj")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify quads with dgo-subj as subject are deleted.
	filter := world.NewGraphQuadWithKeys("dgo-subj", "", "", "")
	quads, err := tx.LookupGraphQuads(ctx, filter, 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(quads) != 0 {
		t.Fatalf("expected 0 quads with dgo-subj as subject after DeleteGraphObject, got %d", len(quads))
	}

	// Verify quads with dgo-subj as object are also deleted.
	filter2 := world.NewGraphQuadWithKeys("", "", "dgo-subj", "")
	quads2, err := tx.LookupGraphQuads(ctx, filter2, 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(quads2) != 0 {
		t.Fatalf("expected 0 quads with dgo-subj as object after DeleteGraphObject, got %d", len(quads2))
	}
}

// TestSDKEngine_ReadOnlyTransaction tests that read-only transactions work.
func TestSDKEngine_ReadOnlyTransaction(t *testing.T) {
	// Start an SDK engine for read-only object access.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// First create an object with a write transaction.
	wtx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	var createdObject world.ObjectState
	createdObject, err = wtx.CreateObject(ctx, "readonly-obj", nil)
	world.ReleaseObjectState(createdObject)
	if err != nil {
		wtx.Discard()
		t.Fatal(err.Error())
	}
	err = wtx.Commit(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Now read with a read-only transaction.
	rtx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rtx.Discard()

	// Read the committed object through the read-only transaction.
	obj, found, err := rtx.GetObject(ctx, "readonly-obj")
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.Fatal("expected object found in read-only tx")
	}
	if obj.GetKey() != "readonly-obj" {
		t.Fatalf("expected key readonly-obj, got %q", obj.GetKey())
	}
}

// TestSDKEngine_WorldEngineInterface verifies the type assertion compiles.
func TestSDKEngine_WorldEngineInterface(t *testing.T) {
	// This test just verifies at compile time that SDKEngine implements world.Engine.
	var _ world.Engine = (*sdk_world_engine.SDKEngine)(nil)
}

// TestSDKEngine_WorldStateInterface verifies the WorldState type assertion compiles.
func TestSDKEngine_WorldStateInterface(t *testing.T) {
	var _ world.WorldState = (*sdk_world_engine.SDKWorldState)(nil)
}

// TestSDKEngine_TxInterface verifies the Tx type assertion compiles.
func TestSDKEngine_TxInterface(t *testing.T) {
	var _ world.Tx = (*sdk_world_engine.SDKTx)(nil)
}

// TestSDKEngine_ObjectStateInterface verifies the ObjectState type assertion compiles.
func TestSDKEngine_ObjectStateInterface(t *testing.T) {
	var _ world.ObjectState = (*sdk_world_engine.SDKObjectState)(nil)
}

// TestSDKEngine_ObjectIteratorInterface verifies the ObjectIterator type assertion compiles.
func TestSDKEngine_ObjectIteratorInterface(t *testing.T) {
	var _ world.ObjectIterator = (*sdk_world_engine.SDKObjectIterator)(nil)
}

// TestSDKEngine_SeqnoAfterOperations verifies seqno tracking across operations.
func TestSDKEngine_SeqnoAfterOperations(t *testing.T) {
	// Start an SDK engine for sequence tracking across commits.
	ctx := context.Background()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()

	// Read the sequence number before either transaction commits.
	s0, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Commit a transaction with object creation.
	tx1, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	var createdObject world.ObjectState
	createdObject, err = tx1.CreateObject(ctx, "seqno-a", nil)
	world.ReleaseObjectState(createdObject)
	if err != nil {
		tx1.Discard()
		t.Fatal(err.Error())
	}
	err = tx1.Commit(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Confirm the first commit advances the sequence number.
	s1, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if s1 <= s0 {
		t.Fatalf("expected seqno to advance after first commit: %d <= %d", s1, s0)
	}

	// Commit another transaction.
	tx2, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	var createdObject2 world.ObjectState
	createdObject2, err = tx2.CreateObject(ctx, "seqno-b", nil)
	world.ReleaseObjectState(createdObject2)
	if err != nil {
		tx2.Discard()
		t.Fatal(err.Error())
	}
	err = tx2.Commit(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Confirm the second commit advances the sequence number again.
	s2, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if s2 <= s1 {
		t.Fatalf("expected seqno to advance after second commit: %d <= %d", s2, s1)
	}

	// Record the sequence progression across both commits.
	t.Logf("seqno progression: %d -> %d -> %d", s0, s1, s2)
}

// TestSDKWorldStateObjectAccessWorldStateInvokesCallback pins that the Go SDK
// ObjectState wraps the returned cursor resource and drives the callback,
// rather than releasing it and returning a silent no-op.
func TestSDKWorldStateObjectAccessWorldStateInvokesCallback(t *testing.T) {
	// Start a testbed World and acquire its root resource client.
	ctx := context.Background()
	_, resClient, tbCleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	defer tbCleanup()
	srpcClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the World and obtain its engine resource reference.
	testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
	createResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	engineRef := resClient.CreateResourceReference(createResp.GetResourceId())
	defer engineRef.Release()
	engineSrpc, err := engineRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open a writable transaction through the World Engine resource.
	engineService := s4wave_world.NewSRPCEngineResourceServiceClient(engineSrpc)
	txResp, err := engineService.NewTransaction(ctx, &s4wave_world.NewTransactionRequest{Write: true})
	if err != nil {
		t.Fatal(err.Error())
	}
	txRef := resClient.CreateResourceReference(txResp.GetResourceId())
	defer txRef.Release()

	// Wrap the transaction as an SDK WorldState and create the test object.
	ws, err := s4wave_world.NewWorldState(resClient, txRef, txResp.GetReadOnly())
	if err != nil {
		t.Fatal(err.Error())
	}
	obj, err := ws.CreateObject(ctx, "sdk-obj-access-world-state", nil)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify ObjectState invokes the callback with its cursor resource.
	called := false
	err = obj.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		called = true
		if cursor == nil {
			t.Fatal("callback received a nil cursor")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if !called {
		t.Fatal("AccessWorldState returned without invoking the callback")
	}
}
