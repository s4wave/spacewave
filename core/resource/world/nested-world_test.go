//go:build !js

package resource_world_test

import (
	"context"
	"maps"
	"testing"

	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// TestOpenNestedWorldResourceRelease checks that the registered resource owns
// the snapshot after its outer transaction ends and releases it on request.
func TestOpenNestedWorldResourceRelease(t *testing.T) {
	// Publish one nested and one ordinary typed object.
	ctx := t.Context()
	tb, cleanup := setupWorldTestbed(ctx, t)
	defer cleanup()

	snapshot, err := world_block.ImportSnapshot(ctx, tb.Engine, maps.All(map[string]block.Block{
		"inner": block_mock.NewExample("content"),
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	const key = "other-app/outer"
	const ordinaryKey = "other-app/ordinary"
	if err := world.ExecTransaction(ctx, tb.Engine, true, func(ctx context.Context, state world.WorldState) error {
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
		if err := world_types.SetObjectType(ctx, state, key, "other-app/type"); err != nil {
			return err
		}
		_, _, err = world.AccessWorldObject(ctx, state, ordinaryKey, true, func(cursor *block.Cursor) error {
			cursor.SetBlock(block_mock.NewExample("ordinary"), true)
			return nil
		})
		if err != nil {
			return err
		}
		return world_types.SetObjectType(ctx, state, ordinaryKey, "other-app/type")
	}); err != nil {
		t.Fatal(err)
	}

	// The resource owner must not retain a handle for rejected opens.
	tx, err := tb.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	resources := &worldStateOperationResourceContext{ctx: ctx}
	resourceCtx := resource_server.WithResourceClientContext(ctx, resources)
	r := resource_world.NewEngineWorldStateResource(tb.Logger, nil, tx, nil, tb.Engine)
	if _, err := r.OpenNestedWorld(resourceCtx, &s4wave_world.OpenNestedWorldRequest{ObjectKey: ordinaryKey}); err == nil {
		t.Fatal("opened ordinary typed object as nested World")
	}
	if len(resources.releases) != 0 {
		t.Fatalf("failed open retained %d resources", len(resources.releases))
	}
	resp, err := r.OpenNestedWorld(resourceCtx, &s4wave_world.OpenNestedWorldRequest{ObjectKey: key})
	tx.Discard()
	if err != nil {
		t.Fatal(err)
	}
	if len(resources.releases) != 1 {
		t.Fatalf("nested resources = %d, want 1", len(resources.releases))
	}
	// The nested snapshot remains readable after its outer transaction ends.
	client, err := resources.GetAttachedResource(resp.GetResourceId())
	if err != nil {
		t.Fatal(err)
	}
	service := s4wave_world.NewSRPCWorldStateResourceServiceClient(client)
	readOnly, err := service.GetReadOnly(resourceCtx, &s4wave_world.GetReadOnlyRequest{})
	if err != nil || !readOnly.GetReadOnly() {
		t.Fatalf("nested read-only state: %v, %v", readOnly, err)
	}
	if _, err := service.CreateObject(resourceCtx, &s4wave_world.CreateObjectRequest{ObjectKey: "forbidden"}); err == nil {
		t.Fatal("created object in nested snapshot")
	}
	if _, err := service.GetSeqno(resourceCtx, &s4wave_world.GetSeqnoRequest{}); err != nil {
		t.Fatalf("nested state after outer discard: %v", err)
	}
	// Releasing the registered resource runs its snapshot cleanup.
	if !resources.ReleaseResource(resp.GetResourceId()) || len(resources.releases) != 0 {
		t.Fatalf("nested resource not released: %d retained", len(resources.releases))
	}
}
