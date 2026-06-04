package world_test

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

func TestAccessObjectReturnsStorageOpArgs(t *testing.T) {
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	ref, err := world.AccessObject(ctx, ws.AccessWorldState, nil, func(bcs *block.Cursor) error {
		bcs.SetBlock(block_mock.NewExample("root"), true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref.GetBucketId() != wtb.EngineBucketID {
		t.Fatalf("expected bucket id %q, got %q", wtb.EngineBucketID, ref.GetBucketId())
	}
	if ref.GetTransformConf().GetEmpty() {
		t.Fatal("expected object ref to retain the storage transform config")
	}

	example, err := world.LookupObjectRef[*block_mock.Example](ctx, ws.AccessWorldState, ref, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err)
	}
	if example.GetMsg() != "root" {
		t.Fatalf("expected root block message, got %q", example.GetMsg())
	}
}

func TestLookupObjectBodyReleasesObjectState(t *testing.T) {
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	_, _, err = world.CreateWorldObject(ctx, ws, "example/body-release", func(bcs *block.Cursor) error {
		bcs.SetBlock(block_mock.NewExample("body"), true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	wrapped := &releaseCountingWorldState{WorldState: ws}
	example, err := world.LookupObjectBody[*block_mock.Example](
		ctx,
		wrapped,
		"example/body-release",
		block_mock.NewExampleBlock,
	)
	if err != nil {
		t.Fatal(err)
	}
	if example.GetMsg() != "body" {
		t.Fatalf("expected body message, got %q", example.GetMsg())
	}
	if wrapped.releases != 1 {
		t.Fatalf("expected lookup to release object state once, got %d", wrapped.releases)
	}
}

type releaseCountingWorldState struct {
	world.WorldState
	releases int
}

func (ws *releaseCountingWorldState) GetObject(
	ctx context.Context,
	key string,
) (world.ObjectState, bool, error) {
	obj, found, err := ws.WorldState.GetObject(ctx, key)
	if err != nil || !found {
		return obj, found, err
	}
	return &releaseCountingObjectState{
		ObjectState: obj,
		releases:    &ws.releases,
	}, true, nil
}

type releaseCountingObjectState struct {
	world.ObjectState
	releases *int
}

func (obj *releaseCountingObjectState) Release() {
	*obj.releases += 1
}
