package world_block_test

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
)

// TestWalkBlocksCopiesObjectDescendants uses separate stores and an opaque World
// object root. Copying metadata alone cannot recover the nested payload.
func TestWalkBlocksCopiesObjectDescendants(t *testing.T) {
	t.Run("with changelog", func(t *testing.T) { testWalkBlocksCopiesObjectDescendants(t, false) })
	t.Run("without changelog", func(t *testing.T) { testWalkBlocksCopiesObjectDescendants(t, true) })
}

func testWalkBlocksCopiesObjectDescendants(t *testing.T, disableChangelog bool) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	source, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := source.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cursor.Release()
	if disableChangelog {
		root, _, err := block.PutBlock(ctx, cursor.GetBucket(), world_block.NewWorld(true))
		if err != nil {
			t.Fatal(err)
		}
		cursor.SetRootRef(root)
	}
	target, err := receiver.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Release()
	ws, err := world_block.BuildMockWorldState(ctx, le, true, cursor, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Discard()

	// These writes use a separate storage cursor, as file imports do, so the
	// World's GC journal cannot supply their outgoing block references.
	leaf, _, err := block.PutBlock(ctx, cursor.GetBucket(), &block_mock.Example{Msg: "durable nested payload"})
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := block.PutBlock(ctx, cursor.GetBucket(), &block_mock.Root{ExampleSubBlock: &block_mock.SubBlock{ExamplePtr: leaf}})
	if err != nil {
		t.Fatal(err)
	}
	{
		createdObject, err := ws.CreateObject(ctx, "content", &bucket.ObjectRef{RootRef: root})
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := world_types.SetObjectType(ctx, ws, "content", "test/nested"); err != nil {
		t.Fatal(err)
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	resolve := func(context.Context, string) (block.Ctor, error) { return block_mock.NewRootBlock, nil }
	copyBlock := func(ref *block.BlockRef, data []byte) error {
		_, _, err := target.GetBucket().PutBlock(ctx, data, &block.PutOpts{ForceBlockRef: ref})
		return err
	}
	if err := ws.WalkBlocks(ctx, resolve, copyBlock); err != nil {
		t.Fatal(err)
	}
	if _, err := target.GetBucket().Sync(ctx); err != nil {
		t.Fatal(err)
	}
	data, found, err := target.GetBucket().GetBlock(ctx, leaf)
	if err != nil || !found {
		t.Fatalf("copied descendant missing: %v", err)
	}
	value := &block_mock.Example{}
	if err := value.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}
	if value.GetMsg() != "durable nested payload" {
		t.Fatalf("unexpected payload: %q", value.GetMsg())
	}

	// An unchanged graph needs no repeated payload copies or object decoding.
	complete := make(map[string]bool)
	options := &world_block.WalkBlocksOptions{
		Known: func(domain string, ref *block.BlockRef) (bool, error) {
			return complete[domain+ref.MarshalString()], nil
		},
		Complete: func(domain string, ref *block.BlockRef) error {
			complete[domain+ref.MarshalString()] = true
			return nil
		},
	}
	if err := ws.WalkBlocks(ctx, resolve, copyBlock, options); err != nil {
		t.Fatal(err)
	}
	if err := ws.WalkBlocks(ctx, resolve, func(*block.BlockRef, []byte) error {
		t.Error("unchanged completed graph copied again")
		return nil
	}, options); err != nil {
		t.Fatal(err)
	}

	if err := ws.WalkBlocks(ctx, func(context.Context, string) (block.Ctor, error) { return nil, nil }, copyBlock); err == nil {
		t.Fatal("unknown object decoder must prevent completion")
	}
	if err := cursor.GetBucket().RmBlock(ctx, leaf); err != nil {
		t.Fatal(err)
	}
	clear(complete)
	if err := ws.WalkBlocks(ctx, resolve, copyBlock, options); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("missing descendant must prevent completion: %v", err)
	}
	if complete["type/test/nested"+root.MarshalString()] {
		t.Fatal("failed descendant retained a completion claim for its parent")
	}
}
