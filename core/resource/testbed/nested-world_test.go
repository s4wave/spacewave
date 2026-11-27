//go:build !js

package resource_testbed

import (
	"context"
	"maps"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// TestNestedWorldResource exercises the nested World Engine through the real
// resource connection: read-only transactions, following the outer object's
// revisions, snapshot retention, and independent release.
func TestNestedWorldResource(t *testing.T) {
	// Connect to a real in-memory ResourceServer and World engine.
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	client, cleanup := SetupResourceClient(ctx, t, tb)
	defer cleanup()

	// Create the outer World through the testbed RPC service.
	root := client.AccessRootResource()
	defer root.Release()
	rootClient, err := root.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	created, err := s4wave_testbed.NewSRPCTestbedResourceServiceClient(rootClient).CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
	if err != nil {
		t.Fatal(err)
	}

	// Wrap the outer World engine with the resource SDK.
	engineRef := client.CreateResourceReference(created.GetResourceId())
	engine, err := s4wave_world.NewEngine(client, engineRef)
	if err != nil {
		engineRef.Release()
		t.Fatal(err)
	}
	defer engine.Release()

	// Wrap the outer World storage with the engine SDK.
	storageRef := client.CreateResourceReference(created.GetResourceId())
	storage, err := sdk_world_engine.NewSDKEngine(client, storageRef)
	if err != nil {
		storageRef.Release()
		t.Fatal(err)
	}
	defer storage.Release()

	// Publish a typed outer object with a standard nested-World root.
	const outerKey = "example/nested"
	publishNestedWorld(ctx, t, storage, tb.EngineBucketID, outerKey, "nested content")

	// Open the nested Engine, then retire the enclosing transaction.
	outer, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	nested, err := outer.OpenNestedWorld(ctx, outerKey)
	outer.Release()
	if err != nil {
		t.Fatal(err)
	}

	// A nested transaction is read-only even when a writer is requested.
	first, err := nested.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	if !first.GetReadOnly() {
		t.Fatal("nested transaction is writable")
	}
	requireExampleBody(ctx, t, first, "nested content")

	// Every World mutation must be rejected by the nested transaction.
	if obj, err := first.CreateObject(ctx, "forbidden", nil); err == nil {
		world.ReleaseObjectState(obj)
		t.Fatal("nested object creation succeeded")
	} else {
		world.ReleaseObjectState(obj)
	}
	if _, err := first.DeleteObject(ctx, "inner"); err == nil {
		t.Fatal("nested object deletion succeeded")
	}
	if err := first.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("inner", "<forbidden>", "inner", "")); err == nil {
		t.Fatal("nested graph mutation succeeded")
	}

	// Republishing the outer object advances the nested Engine.
	seqno, err := first.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	publishNestedWorld(ctx, t, storage, tb.EngineBucketID, outerKey, "updated content")
	if _, err := nested.WaitSeqno(ctx, seqno+1); err != nil {
		t.Fatal(err)
	}

	// A new transaction reads the update, but the first keeps its snapshot.
	second, err := nested.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	requireExampleBody(ctx, t, second, "updated content")
	second.Release()
	requireExampleBody(ctx, t, first, "nested content")

	// The WorldState interface adapter grants the same following Engine.
	adapterTx, err := storage.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	adapter, ok := adapterTx.(*sdk_world_engine.SDKTx)
	if !ok {
		adapterTx.Discard()
		t.Fatal("unexpected World transaction implementation")
	}

	// Open the nested World through the adapter transaction.
	adapterNested, err := adapter.OpenNestedWorld(ctx, outerKey)
	adapterTx.Discard()
	if err != nil {
		t.Fatal(err)
	}

	// The adapter's nested World reads the latest publication.
	err = world.ExecTransaction(ctx, adapterNested, false, func(ctx context.Context, state world.WorldState) error {
		requireExampleBody(ctx, t, state, "updated content")
		return nil
	})
	adapterNested.Release()
	if err != nil {
		t.Fatal(err)
	}

	// Release the nested Engine while the connection and engine stay open.
	nested.Release()
	check, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Release()
	if obj, found, err := check.GetObject(ctx, outerKey); err != nil || !found {
		world.ReleaseObjectState(obj)
		t.Fatalf("connection after release: found %v, error %v", found, err)
	} else {
		world.ReleaseObjectState(obj)
	}
}

// publishNestedWorld sets key in storage to a typed NestedWorld whose "inner"
// object holds msg.
func publishNestedWorld(ctx context.Context, t *testing.T, storage world.Engine, bucketID, key, msg string) {
	// Report failures at the caller.
	t.Helper()

	// Stage the snapshot until the outer object adopts it.
	stage, err := storage.StageWorldState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer stage.Release()
	snapshot, err := world_block.ImportSnapshot(ctx, stage, maps.All(map[string]block.Block{
		"inner": block_mock.NewExample(msg),
	}), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Point the typed outer object at the snapshot.
	err = world.ExecTransaction(ctx, storage, true, func(ctx context.Context, state world.WorldState) error {
		_, _, err := world.AccessWorldObject(ctx, state, key, true, func(cursor *block.Cursor) error {
			nested, err := world_block.NewNestedWorld(bucketID, snapshot, nil)
			if err != nil {
				return err
			}
			cursor.SetBlock(nested, true)
			return nil
		})
		if err != nil {
			return err
		}
		return world_types.SetObjectType(ctx, state, key, "example/custom")
	})
	if err != nil {
		t.Fatal(err)
	}
}

// objectGetter looks up World objects by key.
type objectGetter interface {
	GetObject(ctx context.Context, key string) (world.ObjectState, bool, error)
}

// requireExampleBody fails unless the "inner" object in ws holds msg.
func requireExampleBody(ctx context.Context, t *testing.T, ws objectGetter, msg string) {
	// Report failures at the caller.
	t.Helper()

	// Look up the object, releasing its state after the read.
	obj, found, err := ws.GetObject(ctx, "inner")
	defer world.ReleaseObjectState(obj)
	if err != nil || !found {
		t.Fatalf("nested lookup: found %v, error %v", found, err)
	}

	// Decode its body and compare the message.
	var body *block_mock.Example
	_, _, err = world.AccessObjectState(ctx, obj, false, func(cursor *block.Cursor) error {
		var decodeErr error
		body, decodeErr = block.UnmarshalBlock[block_mock.Example](ctx, cursor)
		return decodeErr
	})
	if err != nil || body.GetMsg() != msg {
		t.Fatalf("nested read: body %v, error %v, want %q", body, err, msg)
	}
}
