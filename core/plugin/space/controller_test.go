package plugin_space

import (
	"context"
	"testing"
	"time"

	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/keyed"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// Readiness test plugin identifiers.
const (
	pluginReadinessPluginID   = "test-readiness-plugin"
	pluginReadinessPlatformID = "test/platform"
	pluginReadinessTypeID     = "test/readiness"
)

func TestReconcileProcessConfigsClearsWhenStoreUnavailable(t *testing.T) {
	// Seed the controller with a stale process configuration.
	c := &Controller{
		processConfigs: map[string]processConfig{
			"stale-process": {typeID: "test/process"},
		},
	}
	c.processes = keyed.NewKeyed(func(key string) (keyed.Routine, processConfig) {
		return nil, c.processConfigs[key]
	})
	c.processes.SyncKeys([]string{"stale-process"}, false)

	// Clear process configurations when their backing store is unavailable.
	c.reconcileProcessConfigs(logrus.NewEntry(logrus.New()), nil)

	// Verify reconciliation removes the stale key and stored process config.
	if got := c.processes.GetKeys(); len(got) != 0 {
		t.Fatalf("process keys after unavailable store = %v, want none", got)
	}
	if got := len(c.processConfigs); got != 0 {
		t.Fatalf("process config count after unavailable store = %d, want 0", got)
	}
}

func TestLookupObjectTypeWaitsForDesiredPluginRegistration(t *testing.T) {
	// Start a testbed with enough time for the plugin registration lifecycle.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// The Space approves one plugin whose Manifest is stored in the Space World.
	if _, _, err := space_world_ops.SetSpaceSettings(
		ctx,
		tb.WorldState,
		"",
		space_world_ops.DefaultSpaceSettingsObjectKey,
		&space_world.SpaceSettings{PluginIds: []string{pluginReadinessPluginID}},
		true,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}
	storeKey := "test/plugin-manifests"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, tb.WorldState, storeKey); err != nil {
		t.Fatal(err)
	}
	if err := bldr_manifest_world.ExStoreManifestOp(
		ctx,
		tb.WorldState,
		tb.Volume.GetPeerID(),
		"test/manifests/readiness",
		[]string{storeKey},
		createReadinessManifest(t, ctx, tb),
	); err != nil {
		t.Fatal(err)
	}

	// The manifest source serves nothing, so the plugin must resolve its
	// Manifest from the Space World.
	// Register a source that leaves the Space World as the manifest provider.
	source, _, err := controllerbus_core.NewCoreBus(ctx, tb.Logger)
	if err != nil {
		t.Fatal(err)
	}

	// Wire the manifest source into the resolver and install the readiness loader.
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus, WithManifestSource(source)))
	loader := newPluginReadinessLoadController(tb.Bus)
	releaseLoader, err := tb.Bus.AddController(ctx, loader, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseLoader()
	_, _, spaceRef, err := StartControllerWithConfig(ctx, tb.Bus, &Config{
		SpaceId:       "space-test",
		VolumeId:      tb.EngineVolumeID,
		ObjectStoreId: tb.EngineObjectStoreID,
		EngineId:      tb.EngineID,
		SessionPeerId: tb.Volume.GetPeerID().String(),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer spaceRef.Release()

	// Wait for the approved plugin to begin loading and resolve its Manifest.
	select {
	case <-loader.loadStarted:
	case <-ctx.Done():
		t.Fatal("Space settings did not start the desired plugin")
	}
	select {
	case <-loader.manifestResolved:
	case <-ctx.Done():
		t.Fatal("desired plugin did not resolve the imported Manifest")
	}

	// A typed lookup waits until the desired plugin registers its type.
	type lookupResult struct {
		objectType objecttype.ObjectType
		ref        directive.Reference
		err        error
	}

	// Start a typed lookup while the desired plugin is still registering.
	lookupCtx := objecttype.WithEngineID(ctx, tb.EngineID)
	lookupResultCh := make(chan lookupResult, 1)
	go func() {
		ot, ref, err := objecttype.ExLookupObjectType(lookupCtx, tb.Bus, pluginReadinessTypeID)
		lookupResultCh <- lookupResult{objectType: ot, ref: ref, err: err}
	}()
	select {
	case result := <-lookupResultCh:
		if result.ref != nil {
			result.ref.Release()
		}
		t.Fatalf("typed lookup completed before registration: objectType=%v err=%v", result.objectType, result.err)
	case <-loader.lookupObserved:
	}

	// Allow registration and wait for the typed lookup result.
	close(loader.allowRegistration)
	var result lookupResult
	select {
	case result = <-lookupResultCh:
	case <-ctx.Done():
		t.Fatal("typed lookup did not resolve after registration")
	}
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.objectType == nil || result.objectType.GetObjectTypeID() != pluginReadinessTypeID {
		t.Fatalf("typed lookup returned %v", result.objectType)
	}
	if result.ref == nil {
		t.Fatal("typed lookup returned no reference")
	}
	result.ref.Release()

	// Once every desired plugin is loaded, an unknown type resolves to nothing.
	unknown, unknownRef, err := objecttype.ExLookupObjectType(lookupCtx, tb.Bus, "test/unknown-readiness")
	if unknownRef != nil {
		unknownRef.Release()
	}
	if err != nil {
		t.Fatal(err)
	}
	if unknown != nil {
		t.Fatalf("unknown typed lookup returned %v", unknown)
	}
}

// createReadinessManifest retains the readiness plugin Manifest in the test
// World's bucket.
func createReadinessManifest(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
) *bldr_manifest.ManifestRef {
	// Describe the readiness manifest.
	t.Helper()
	meta := bldr_manifest.NewManifestMeta(
		pluginReadinessPluginID,
		bldr_manifest.BuildType_DEV,
		pluginReadinessPlatformID,
		1,
	)

	// Stage the manifest until the test ends so the caller can adopt it.
	stage, err := tb.Engine.StageWorldState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stage.Release)
	var manifestRef *bldr_manifest.ManifestRef
	if err := stage.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		// Write the manifest block.
		transaction, blocks := cursor.BuildTransactionAtRef(nil, nil)
		blocks.SetBlock(bldr_manifest.NewManifest(meta, "entrypoint"), true)
		rootRef, _, err := transaction.Write(ctx, true)
		if err != nil {
			return err
		}

		// Point the manifest ref at the written root.
		objectRef := cursor.GetRef().CloneVT()
		objectRef.RootRef = rootRef
		manifestRef = bldr_manifest.NewManifestRef(meta, objectRef)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return manifestRef
}
