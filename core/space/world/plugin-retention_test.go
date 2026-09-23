package space_world_test

import (
	"context"
	"testing"

	manifest "github.com/s4wave/spacewave/bldr/manifest"
	builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	"github.com/s4wave/spacewave/bldr/manifest/builder/resultworld"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/byteslice"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/identity"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/sirupsen/logrus"
)

// TestPluginWorldRetention traverses app bindings and the complete built and
// source trees without a running plugin or an editable source object.
func TestPluginWorldRetention(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cursor.Release()
	ws, err := world_block.BuildMockWorldState(ctx, le, true, cursor, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Discard()
	put := func(value block.Block) *block.BlockRef {
		t.Helper()
		ref, _, err := block.PutBlock(ctx, cursor.GetBucket(), value)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	leaf := put(&unixfs_block.FSNode{NodeType: unixfs_block.NodeType_NodeType_FILE})
	sourceLeaf := put(&unixfs_block.FSNode{NodeType: unixfs_block.NodeType_NodeType_FILE, Permissions: 0o644})
	source := put(&unixfs_block.FSNode{
		NodeType:       unixfs_block.NodeType_NodeType_DIRECTORY,
		DirectoryEntry: []*unixfs_block.Dirent{{Name: "app.ts", NodeRef: sourceLeaf}},
	})
	assets := put(&unixfs_block.FSNode{
		NodeType:       unixfs_block.NodeType_NodeType_DIRECTORY,
		DirectoryEntry: []*unixfs_block.Dirent{{Name: "plugin.mjs", NodeRef: leaf}},
	})
	binding := []byte(`{"application":"colors/votes","version":1}`)
	objects := []struct {
		key, typeID string
		value       block.Block
	}{
		{"manifest", manifest_world.ManifestTypeID, &manifest.Manifest{AssetsFsRef: assets}},
		{"result", resultworld.ManifestBuildResultTypeID, &builder.BuilderResult{SourceRef: &bucket.ObjectRef{RootRef: source}}},
		{"app", "sync/app/colors/votes", byteslice.NewByteSlice(&binding)},
		{"worker-key", identity_world.KeypairTypeID, &identity.Keypair{PeerId: "worker"}},
	}
	for _, entry := range objects {
		object, err := ws.CreateObject(ctx, entry.key, &bucket.ObjectRef{RootRef: put(entry.value)})
		world.ReleaseObjectState(object)
		if err != nil {
			t.Fatal(err)
		}
		if err := world_types.SetObjectType(ctx, ws, entry.key, entry.typeID); err != nil {
			t.Fatal(err)
		}
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	visited := make(map[string]bool)
	err = ws.WalkBlocks(ctx, func(ctx context.Context, typeID string) (block.Ctor, error) {
		decoder, err := space_world.LookupBlockType(ctx, typeID)
		if err != nil || decoder == nil {
			return nil, err
		}
		return decoder.Constructor, nil
	}, func(ref *block.BlockRef, _ []byte, _ []*block.BlockRef) error {
		visited[ref.MarshalString()] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []*block.BlockRef{leaf, sourceLeaf, source, assets} {
		if !visited[ref.MarshalString()] {
			t.Fatalf("retention omitted plugin dependency %s", ref.MarshalString())
		}
	}
}
