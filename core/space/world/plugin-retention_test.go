package space_world_test

import (
	"testing"

	manifest "github.com/s4wave/spacewave/bldr/manifest"
	builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	"github.com/s4wave/spacewave/bldr/manifest/builder/resultworld"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/byteslice"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	"github.com/s4wave/spacewave/db/bucket"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/testbed"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/identity"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// TestPluginWorldRetention checks that a graph copy of a plugin World reaches
// app bindings and the complete built and source trees from recorded edges,
// with no block types or running plugin.
func TestPluginWorldRetention(t *testing.T) {
	// Open a block storage testbed for the plugin World.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Open an empty bucket cursor to hold the plugin World blocks.
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cursor.Release()

	// Construct a writable World in the test bucket.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, cursor, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Discard()

	// Provide a block writer for the plugin dependency fixtures.
	put := func(value block.Block) *block.BlockRef {
		t.Helper()
		ref, _, err := block.PutBlock(ctx, cursor.GetBucket(), value)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}

	// Store the plugin source and built asset trees.
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

	// Create the manifest, build result, app binding, and worker key objects.
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
		// Create the dependency object and release its World handle.
		object, err := ws.CreateObject(ctx, entry.key, &bucket.ObjectRef{RootRef: put(entry.value)})
		world.ReleaseObjectState(object)
		if err != nil {
			t.Fatal(err)
		}

		// Record the dependency object type in the World graph.
		if err := world_types.SetObjectType(ctx, ws, entry.key, entry.typeID); err != nil {
			t.Fatal(err)
		}
	}

	// Commit the plugin World before copying its reachable block graph.
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Copy the plugin World graph to a new store and record the visited blocks.
	visited := make(map[string]bool)
	dst := block_store_inmem.NewInmemBlock(
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		hash.RecommendedHashType,
		false,
	)
	err = block.CopyGraph(ctx, cursor.GetBucket(), dst, ws.GetRootRef(), &block.GraphCopyOptions{
		Visited: func(ref *block.BlockRef, _ []byte) {
			visited[ref.MarshalString()] = true
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the graph copy retained every source and built asset dependency.
	for _, ref := range []*block.BlockRef{leaf, sourceLeaf, source, assets} {
		if !visited[ref.MarshalString()] {
			t.Fatalf("retention omitted plugin dependency %s", ref.MarshalString())
		}
	}
}
