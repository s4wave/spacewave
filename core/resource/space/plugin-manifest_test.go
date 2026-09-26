package resource_space

import (
	"context"
	"testing"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/testbed"
)

// createSpacePluginManifest retains a distinct manifest in the test World's bucket.
func createSpacePluginManifest(t *testing.T, ctx context.Context, tb *testbed.Testbed, id, platform string, rev uint64) *bldr_manifest.ManifestRef {
	t.Helper()
	meta := bldr_manifest.NewManifestMeta(
		id,
		bldr_manifest.BuildType_DEV,
		platform,
		rev,
	)
	var manifestRef *bldr_manifest.ManifestRef
	if err := tb.Engine.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		transaction, blocks := cursor.BuildTransactionAtRef(nil, nil)
		blocks.SetBlock(bldr_manifest.NewManifest(meta, "entrypoint"), true)
		rootRef, _, err := transaction.Write(ctx, true)
		if err != nil {
			return err
		}
		objectRef := cursor.GetRef().CloneVT()
		objectRef.RootRef = rootRef
		manifestRef = bldr_manifest.NewManifestRef(meta, objectRef)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return manifestRef
}
