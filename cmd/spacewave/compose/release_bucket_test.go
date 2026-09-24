package spacewave_compose

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_project_starlark "github.com/s4wave/spacewave/bldr/project/starlark"
	cdn_world_controller "github.com/s4wave/spacewave/core/cdn/world/controller"
	block_store_bucket "github.com/s4wave/spacewave/db/block/store/bucket"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	"github.com/sirupsen/logrus"
)

// TestReleaseBucketsResolvePublishedAndCachedRefs reads both reference forms
// through the shipped bucket mappings and the node's ordinary lookup path.
func TestReleaseBucketsResolvePublishedAndCachedRefs(t *testing.T) {
	// Evaluate the actual browser bootstrap instead of duplicating its config.
	result, err := bldr_project_starlark.Evaluate(filepath.Join("..", "..", "..", "bldr.star"))
	if err != nil {
		t.Fatal(err)
	}
	entry := result.Config.GetBuild()["release-web"].GetManifestOverrides()["spacewave-launcher"]
	var launcher bldr_plugin_compiler_go.Config
	if err := launcher.UnmarshalJSON(entry.GetConfig()); err != nil {
		t.Fatal(err)
	}
	configs := launcher.GetHostConfigSet()
	var worldConf cdn_world_controller.Config
	if err := worldConf.UnmarshalJSON(configs["release-world"].GetConfig()); err != nil {
		t.Fatal(err)
	}

	// Substitute only the backing bytes, retaining the production lookup path.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	b, err := newComposedBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	storeCtrl := block_store_inmem.NewController(le, &block_store_inmem.Config{
		BlockStoreId: cdn_world_controller.ReleaseBlockStoreID,
	})
	releaseStore, err := b.AddController(ctx, storeCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseStore()
	store, storeRef, err := storeCtrl.WaitBlockStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer storeRef.Release()
	root, _, err := store.PutBlock(ctx, []byte("published manifest"), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, nodeRef, err := loader.WaitExecControllerRunning(ctx, b, resolver.NewLoadControllerWithConfig(&node_controller.Config{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer nodeRef.Release()
	for _, entry := range configs {
		if entry.GetId() != block_store_bucket.ConfigID {
			continue
		}
		var conf block_store_bucket.Config
		if err := conf.UnmarshalJSON(entry.GetConfig()); err != nil {
			t.Fatal(err)
		}
		_, _, ref, err := loader.WaitExecControllerRunning(ctx, b, resolver.NewLoadControllerWithConfig(&conf), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer ref.Release()
	}

	// A published Space ID and the legacy alias must reach the same block.
	for _, bucketID := range []string{worldConf.GetSpaceId(), "spacewave-release"} {
		cursor, err := bucket_lookup.BuildCursor(ctx, b, le, nil, "", &bucket.ObjectRef{
			BucketId: bucketID,
			RootRef:  root,
		}, nil)
		if err != nil {
			t.Fatalf("resolve release bucket %q: %v", bucketID, err)
		}
		data, found, err := cursor.GetBlock(ctx, root)
		cursor.Release()
		if err != nil || !found || string(data) != "published manifest" {
			t.Fatalf("read release bucket %q: data=%q found=%v err=%v", bucketID, data, found, err)
		}
	}
}
