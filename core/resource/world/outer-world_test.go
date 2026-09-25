//go:build !js

package resource_world_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// TestOpenOuterWorld grants the enclosing Space Engine independently of its
// nested snapshot, confines it to that Space, and applies World ops there.
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
		outerWorld, err := sdk_world_engine.NewSDKEngine(client, outer.GetResourceRef())
		if err != nil {
			t.Fatal(err)
		}
		requireObject(ctx, t, outerWorld, key, true)
		requireObject(ctx, t, outerWorld, otherKey, false)

		// World ops applied through the outer Engine commit in the enclosing Space.
		layoutKey := fmt.Sprintf("space/layout-%v", releaseNestedFirst)
		err = world.ExecTransaction(ctx, outerWorld, true, func(ctx context.Context, state world.WorldState) error {
			_, _, err := space_world_ops.InitObjectLayout(ctx, state, "", layoutKey, time.Now())
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		requireObject(ctx, t, storage, layoutKey, true)
		requireObject(ctx, t, other, layoutKey, false)

		// A transaction on the outer Engine is top-level, and its nested World
		// grants the same Space again.
		outerTx, err := outer.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := outerTx.OpenOuterWorld(ctx); err == nil {
			outerTx.Release()
			t.Fatal("reopened outer World from a top-level transaction")
		}
		recursiveNested, err := outerTx.OpenNestedWorld(ctx, key)
		outerTx.Release()
		if err != nil {
			t.Fatal(err)
		}
		recursiveOuter, err := recursiveNested.OpenOuterWorld(ctx)
		recursiveNested.Release()
		if err != nil {
			t.Fatal(err)
		}
		recursiveWorld, err := sdk_world_engine.NewSDKEngine(client, recursiveOuter.GetResourceRef())
		if err != nil {
			t.Fatal(err)
		}
		requireObject(ctx, t, recursiveWorld, layoutKey, true)
		requireObject(ctx, t, recursiveWorld, otherKey, false)
		recursiveOuter.Release()

		if releaseNestedFirst {
			nested.Release()
			requireObject(ctx, t, outerWorld, key, true)
			outer.Release()
		} else {
			outer.Release()
			obj, found, err := nested.GetObject(ctx, "inner")
			world.ReleaseObjectState(obj)
			if err != nil || !found {
				t.Fatalf("nested World after outer release: found %v, err %v", found, err)
			}
			nested.Release()
		}
	}
}

// requireObject fails unless the current state of eng has key exactly when want.
func requireObject(ctx context.Context, t *testing.T, eng world.Engine, key string, want bool) {
	t.Helper()
	var found bool
	err := world.ExecTransaction(ctx, eng, false, func(ctx context.Context, state world.WorldState) error {
		obj, ok, err := state.GetObject(ctx, key)
		world.ReleaseObjectState(obj)
		found = ok
		return err
	})
	if err != nil {
		t.Fatalf("lookup %s: %v", key, err)
	}
	if found != want {
		t.Fatalf("lookup %s: found %v, want %v", key, found, want)
	}
}
