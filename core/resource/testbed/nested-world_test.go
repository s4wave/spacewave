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

// TestNestedWorldResource exercises the published snapshot through the real
// resource connection, including read-only enforcement and independent release.
func TestNestedWorldResource(t *testing.T) {
	// Connect to a real in-memory ResourceServer and World engine.
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	client, cleanup := SetupResourceClient(ctx, t, tb)
	defer cleanup()

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
	engineRef := client.CreateResourceReference(created.GetResourceId())
	engine, err := s4wave_world.NewEngine(client, engineRef)
	if err != nil {
		engineRef.Release()
		t.Fatal(err)
	}
	defer engine.Release()
	storageRef := client.CreateResourceReference(created.GetResourceId())
	storage, err := sdk_world_engine.NewSDKEngine(client, storageRef)
	if err != nil {
		storageRef.Release()
		t.Fatal(err)
	}
	defer storage.Release()

	// Publish a typed outer object with a standard nested-World root.
	snapshot, err := world_block.ImportSnapshot(ctx, storage, maps.All(map[string]block.Block{
		"inner": block_mock.NewExample("nested content"),
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	const outerKey = "example/nested"
	if err := world.ExecTransaction(ctx, storage, true, func(ctx context.Context, state world.WorldState) error {
		_, _, err := world.AccessWorldObject(ctx, state, outerKey, true, func(cursor *block.Cursor) error {
			nested, err := world_block.NewNestedWorld(tb.EngineBucketID, snapshot, nil)
			if err != nil {
				return err
			}
			cursor.SetBlock(nested, true)
			return nil
		})
		if err != nil {
			return err
		}
		return world_types.SetObjectType(ctx, state, outerKey, "example/custom")
	}); err != nil {
		t.Fatal(err)
	}

	// Open the nested state, then retire the enclosing transaction.
	outer, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	nested, err := outer.OpenNestedWorld(ctx, outerKey)
	if err != nil {
		outer.Release()
		t.Fatal(err)
	}
	outer.Release()
	if !nested.GetReadOnly() {
		t.Fatal("nested state is writable")
	}
	// Read the retained snapshot through the nested resource.
	obj, found, err := nested.GetObject(ctx, "inner")
	if err != nil || !found {
		world.ReleaseObjectState(obj)
		t.Fatalf("nested lookup: found %v, error %v", found, err)
	}
	var body *block_mock.Example
	_, _, err = world.AccessObjectState(ctx, obj, false, func(cursor *block.Cursor) error {
		var decodeErr error
		body, decodeErr = block.UnmarshalBlock[*block_mock.Example](ctx, cursor, block_mock.NewExampleBlock)
		return decodeErr
	})
	world.ReleaseObjectState(obj)
	if err != nil || body.GetMsg() != "nested content" {
		t.Fatalf("nested read: body %v, error %v", body, err)
	}
	// Every World mutation must be rejected by the nested state.
	if obj, err := nested.CreateObject(ctx, "forbidden", nil); err == nil {
		world.ReleaseObjectState(obj)
		t.Fatal("nested object creation succeeded")
	} else {
		world.ReleaseObjectState(obj)
	}
	if _, err := nested.DeleteObject(ctx, "inner"); err == nil {
		t.Fatal("nested object deletion succeeded")
	}
	if err := nested.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("inner", "<forbidden>", "inner", "")); err == nil {
		t.Fatal("nested graph mutation succeeded")
	}

	// The WorldState interface adapter must expose the same retained snapshot.
	adapterTx, err := storage.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	adapter, ok := adapterTx.(*sdk_world_engine.SDKTx)
	if !ok {
		adapterTx.Discard()
		t.Fatal("unexpected World transaction implementation")
	}
	adapterNested, err := adapter.OpenNestedWorld(ctx, outerKey)
	adapterTx.Discard()
	if err != nil {
		t.Fatal(err)
	}
	adapterBody, err := world.LookupObjectBody[*block_mock.Example](ctx, adapterNested, "inner", block_mock.NewExampleBlock)
	adapterNested.Release()
	if err != nil || adapterBody.GetMsg() != "nested content" {
		t.Fatalf("WorldState adapter nested read: body %v, error %v", adapterBody, err)
	}

	// Release the nested resource while the connection and engine stay open.
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
