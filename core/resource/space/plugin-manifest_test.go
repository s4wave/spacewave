package resource_space

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_trust "github.com/s4wave/spacewave/bldr/manifest/trust"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/testbed"
)

// createSpacePluginManifest retains a distinct manifest in the test World's bucket.
func createSpacePluginManifest(t *testing.T, ctx context.Context, tb *testbed.Testbed, id, platform string, rev uint64) *bldr_manifest.ManifestRef {
	// Prepare the plugin manifest metadata for the requested artifact.
	t.Helper()
	meta := bldr_manifest.NewManifestMeta(
		id,
		bldr_manifest.BuildType_DEV,
		platform,
		rev,
	)

	// Stage the manifest until the test ends so the caller can adopt it.
	stage, err := tb.Engine.StageWorldState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stage.Release)
	var manifestRef *bldr_manifest.ManifestRef
	if err := stage.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		// Persist the plugin manifest and retain its exact World bucket reference.
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

// admitSpacePluginManifest admits the manifest of plugin id at ref on b until
// the test ends, as the release or the device's person does. The Space loads a
// World manifest only once AdmitPluginRoot admits its root.
func admitSpacePluginManifest(t *testing.T, b bus.Bus, id string, ref *bucket.ObjectRef) {
	t.Helper()
	trusted := bldr_manifest_trust.NewSet()
	trusted.Add(bldr_manifest_trust.Root{PluginID: id, ManifestRoot: ref.GetRootRef().GetHash().MarshalString()})
	addSpaceRuntimeController(t, b, bldr_manifest_trust.NewController(trusted))
}
