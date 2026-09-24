//go:build !js

package resource_world_test

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// TestOpenOuterWorld keeps the enclosing Space capability independent of its
// nested snapshot and never exposes another engine or a writable service.
func TestOpenOuterWorld(t *testing.T) {
	ctx := t.Context()
	tb, releaseTestbed := setupWorldTestbed(ctx, t)
	defer releaseTestbed()
	client, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
	defer cleanup()

	storageRef := client.CreateResourceReference(engine.GetResourceRef().GetResourceID())
	storage, err := sdk_world_engine.NewSDKEngine(client, storageRef)
	if err != nil {
		storageRef.Release()
		t.Fatal(err)
	}
	defer storage.Release()

	snapshot, err := world_block.BuildSnapshot(ctx, tb.Logger, storage, func(ctx context.Context, state *world_block.WorldState) error {
		_, _, err := world.AccessWorldObject(ctx, state, "inner", true, func(cursor *block.Cursor) error {
			cursor.SetBlock(block_mock.NewExample("nested"), true)
			return nil
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	const key = "space/nested"
	err = world.ExecTransaction(ctx, storage, true, func(ctx context.Context, state world.WorldState) error {
		_, _, err := world.AccessWorldObject(ctx, state, key, true, func(cursor *block.Cursor) error {
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
		return world_types.SetObjectType(ctx, state, key, "space/type")
	})
	if err != nil {
		t.Fatal(err)
	}

	// A second engine on the same connection is not the enclosing Space.
	rootRef := client.AccessRootResource()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	otherResp, err := s4wave_testbed.NewSRPCTestbedResourceServiceClient(rootClient).CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
	rootRef.Release()
	if err != nil {
		t.Fatal(err)
	}
	otherRef := client.CreateResourceReference(otherResp.GetResourceId())
	other, err := sdk_world_engine.NewSDKEngine(client, otherRef)
	if err != nil {
		otherRef.Release()
		t.Fatal(err)
	}
	defer other.Release()
	const otherKey = "other-space/secret"
	if err := world.ExecTransaction(ctx, other, true, func(ctx context.Context, state world.WorldState) error {
		obj, err := state.CreateObject(ctx, otherKey, nil)
		world.ReleaseObjectState(obj)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// The root transaction is not the returned outer handle's owner.
	for _, releaseNestedFirst := range []bool{true, false} {
		root, err := engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := root.OpenOuterWorld(ctx); err == nil {
			root.Release()
			t.Fatal("opened outer World from a non-nested resource")
		}
		nested, err := root.OpenNestedWorld(ctx, key)
		root.Release()
		if err != nil {
			t.Fatal(err)
		}
		outer, err := nested.OpenOuterWorld(ctx)
		if err != nil {
			nested.Release()
			t.Fatal(err)
		}
		if !outer.GetReadOnly() {
			t.Fatal("outer World is writable")
		}
		serviceClient, err := outer.GetResourceRef().GetClient()
		if err != nil {
			t.Fatal(err)
		}
		service := s4wave_world.NewSRPCWorldStateResourceServiceClient(serviceClient)
		// The read-only outer handle must not mount the engine's typed-object service.
		typed := s4wave_world.NewSRPCTypedObjectResourceServiceClient(serviceClient)
		if _, err := typed.AccessTypedObject(ctx, &s4wave_world.AccessTypedObjectRequest{ObjectKey: key}); err == nil {
			t.Fatal("outer World exposed typed-object access")
		}
		if _, err := service.CreateObject(ctx, &s4wave_world.CreateObjectRequest{ObjectKey: "forbidden"}); err == nil {
			t.Fatal("outer World accepted a write")
		}
		if _, err := service.SetGraphQuad(ctx, &s4wave_world.SetGraphQuadRequest{Quad: &quad.Quad{Subject: "forbidden", Predicate: "test", Obj: "value"}}); err == nil {
			t.Fatal("outer World accepted graph mutation")
		}
		obj, found, err := outer.GetObject(ctx, key)
		world.ReleaseObjectState(obj)
		if err != nil || !found {
			t.Fatalf("outer World lost enclosing Space object: found %v, err %v", found, err)
		}
		obj, found, err = outer.GetObject(ctx, otherKey)
		world.ReleaseObjectState(obj)
		if err != nil || found {
			t.Fatalf("outer World accessed another Space: found %v, err %v", found, err)
		}
		// The returned outer state can open a nested World without granting
		// OpenOuterWorld directly on that non-nested resource.
		if _, err := outer.OpenOuterWorld(ctx); err == nil {
			t.Fatal("reopened outer World from non-nested read-only state")
		}
		recursiveNested, err := outer.OpenNestedWorld(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		recursiveOuter, err := recursiveNested.OpenOuterWorld(ctx)
		if err != nil {
			recursiveNested.Release()
			t.Fatal(err)
		}
		recursiveNested.Release()
		obj, found, err = recursiveOuter.GetObject(ctx, key)
		world.ReleaseObjectState(obj)
		if err != nil || !found {
			t.Fatalf("recursive outer World after nested release: found %v, err %v", found, err)
		}
		obj, found, err = recursiveOuter.GetObject(ctx, otherKey)
		world.ReleaseObjectState(obj)
		if err != nil || found {
			t.Fatalf("recursive outer World accessed another Space: found %v, err %v", found, err)
		}
		recursiveOuter.Release()

		if releaseNestedFirst {
			nested.Release()
			obj, found, err = outer.GetObject(ctx, key)
			world.ReleaseObjectState(obj)
			if err != nil || !found {
				t.Fatalf("outer World after nested release: found %v, err %v", found, err)
			}
			outer.Release()
		} else {
			outer.Release()
			obj, found, err = nested.GetObject(ctx, "inner")
			world.ReleaseObjectState(obj)
			if err != nil || !found {
				t.Fatalf("nested World after outer release: found %v, err %v", found, err)
			}
			nested.Release()
		}
	}
}
