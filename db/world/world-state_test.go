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
