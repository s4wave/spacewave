//go:build !js && !goscript

package dist_entrypoint

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/go-git/go-billy/v6/memfs"
	bldr_dist_compiler "github.com/s4wave/spacewave/bldr/dist/compiler"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_fetch_world "github.com/s4wave/spacewave/bldr/manifest/fetch/world"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	plugin_entrypoint_controller "github.com/s4wave/spacewave/bldr/plugin/entrypoint/controller"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_controller "github.com/s4wave/spacewave/bldr/plugin/host/controller"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	bldr_project_starlark "github.com/s4wave/spacewave/bldr/project/starlark"
	"github.com/s4wave/spacewave/bldr/testbed"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/sirupsen/logrus"
)

// TestBrowserBootstrapMountsWorldAndColdStartsRemoteCore checks remote startup followed by durable local materialization.
func TestBrowserBootstrapMountsWorldAndColdStartsRemoteCore(t *testing.T) {
	// Bound the complete startup and background-materialization scenario.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	le := logrus.NewEntry(logrus.New())

	// Resolve the real bootstrap configuration before substituting fixture storage.
	result, err := bldr_project_starlark.Evaluate(filepath.Join("..", "..", "..", "bldr.star"))
	if err != nil {
		t.Fatal(err)
	}
	build := result.Config.GetBuild()["release-web-lazy-plugin-fixture"]
	if build == nil {
		t.Fatal("missing browser bootstrap release fixture")
	}
	browserOverride := build.GetManifestOverrides()["spacewave-browser"]
	if browserOverride == nil {
		t.Fatal("missing browser override")
	}
	var distConf bldr_dist_compiler.Config
	if err := distConf.UnmarshalJSON(browserOverride.GetConfig()); err != nil {
		t.Fatalf("verify browser bootstrap embedded config: %v", err)
	}
	embeds := distConf.GetEmbedManifests()
	if len(embeds) != 2 || embeds[0].GetManifestId() != "spacewave-launcher" || embeds[0].GetPlatformId() != "js" ||
		embeds[1].GetManifestId() != "bldr-materializer" || embeds[1].GetPlatformId() != "js" {
		t.Fatalf("embedded manifests = %#v, want launcher and materializer on js", embeds)
	}

	launcherOverride := build.GetManifestOverrides()["spacewave-launcher"]
	if launcherOverride == nil {
		t.Fatal("missing launcher override")
	}
	var launcherConf bldr_plugin_compiler_go.Config
	if err := launcherConf.UnmarshalJSON(launcherOverride.GetConfig()); err != nil {
		t.Fatalf("verify embedded launcher config: %v", err)
	}
	if err := launcherConf.Validate(); err != nil {
		t.Fatalf("validate embedded launcher config: %v", err)
	}
	embeddedFetch := launcherConf.GetHostConfigSet()["release-world-fetch"]
	if embeddedFetch == nil || embeddedFetch.GetId() != manifest_fetch_world.ConfigID {
		t.Fatalf("launcher release-world-fetch config = %#v", embeddedFetch)
	}

	// Exercise the production scheduler over isolated in-memory stores.
	tb, err := testbed.BuildTestbedWithSchedulerConfig(
		ctx,
		le,
		func(engineID, objectKey, volumeID, peerID string) *plugin_host_scheduler.Config {
			return newReleaseSchedulerConfig("spacewave", engineID, objectKey, volumeID, peerID)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	tb.GetStaticResolver().AddFactory(manifest_fetch_world.NewFactory(tb.GetBus()))

	// Publish executable refs in separate buckets that require later copying.
	const releaseObjectKey = "spacewave/release/manifests"
	if _, err := bldr_manifest_world.CreateManifestStoreInEngine(ctx, tb.GetWorldEngine(), releaseObjectKey); err != nil {
		t.Fatal(err)
	}
	coreRef := buildRemoteManifest(t, ctx, tb, le, "spacewave-release", "spacewave-core")
	transientRef := buildRemoteManifest(t, ctx, tb, le, "transient-release-provider", "transient-plugin")
	for _, ref := range []*bldr_manifest.ManifestRef{coreRef, transientRef} {
		manifestKey := bldr_manifest.NewManifestKey(releaseObjectKey, ref.GetMeta())
		if err := bldr_manifest_world.ExStoreManifestOp(
			ctx,
			tb.GetWorldState(),
			tb.GetVolume().GetPeerID(),
			manifestKey,
			[]string{releaseObjectKey},
			ref,
		); err != nil {
			t.Fatal(err)
		}
	}

	// Apply the verified launcher's FetchManifest controller through the normal
	// config-set owner, substituting only the in-memory Release World engine.
	fetchConf := &manifest_fetch_world.Config{}
	if err := fetchConf.UnmarshalJSON(embeddedFetch.GetConfig()); err != nil {
		t.Fatal(err)
	}
	fetchConf.EngineId = tb.GetWorldEngineID()
	fetchConf.ObjectKeys = []string{releaseObjectKey}
	fetchEntry, err := configset_proto.NewControllerConfig(
		configset.NewControllerConfig(embeddedFetch.GetRev(), fetchConf),
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := configset_proto.ConfigSetMap{"release-world-fetch": fetchEntry}.Resolve(ctx, tb.GetBus())
	if err != nil {
		t.Fatalf("resolve launcher Release World config: %v", err)
	}
	_, fetchSetRef, err := tb.GetBus().AddDirective(configset.NewApplyConfigSet(resolved), nil)
	if err != nil {
		t.Fatalf("mount launcher Release World on plugin-host bus: %v", err)
	}
	t.Cleanup(fetchSetRef.Release)

	fetchValue, _, fetchRef, err := bus.ExecWaitValue[*bldr_manifest.FetchManifestValue](
		ctx,
		tb.GetBus(),
		bldr_manifest.NewFetchManifest("spacewave-core", nil, []string{"js"}, 0),
		nil,
		nil,
		func(value *bldr_manifest.FetchManifestValue) (bool, error) {
			return len(value.GetManifestRefs()) != 0, nil
		},
	)
	if err != nil {
		t.Fatalf("launcher Release World FetchManifest: %v", err)
	}
	t.Cleanup(fetchRef.Release)
	if len(fetchValue.GetManifestRefs()) != 1 || !fetchValue.GetManifestRefs()[0].EqualVT(coreRef) {
		t.Fatalf("FetchManifest refs = %#v, want remote Core", fetchValue.GetManifestRefs())
	}

	// Hold the background copy gate until configured startup is admitted.
	startupSource := newReleaseFixtureStartupSource()
	startupGroup := plugin_entrypoint_controller.NewStartupGroupCoordinator(
		[]string{"configured-startup-plugin"},
		startupSource,
	)
	tb.GetScheduler().SetManifestCopyGate(startupGroup)

	// Observe executable admission without requiring a JavaScript runtime.
	host := &releaseFixturePluginHost{started: make(chan string, 4)}
	hostCtrl := plugin_host_controller.NewController(
		le,
		tb.GetBus(),
		controller.NewInfo("test/release-core-host", controller.MustParseVersion("0.0.1"), "release fixture host"),
		host,
	)
	releaseHost, err := tb.GetBus().AddController(ctx, hostCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseHost)

	// Start Core from its remote bucket while background copying is gated.
	_, loadRef, err := tb.GetBus().AddDirective(bldr_plugin.NewLoadPlugin("spacewave-core"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(loadRef.Release)
	if err := startupGroup.Start(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case pluginID := <-host.started:
		if pluginID != "spacewave-core" {
			t.Fatalf("scheduler started %q, want spacewave-core", pluginID)
		}
	case <-ctx.Done():
		t.Fatalf("scheduler did not cold-start remote Core: %v", ctx.Err())
	}

	// Apply the same deferred-copy rule to an unconfigured runtime plugin.
	_, transientLoadRef, err := tb.GetBus().AddDirective(bldr_plugin.NewLoadPlugin("transient-plugin"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(transientLoadRef.Release)
	select {
	case pluginID := <-host.started:
		if pluginID != "transient-plugin" {
			t.Fatalf("scheduler started %q, want transient-plugin", pluginID)
		}
	case <-ctx.Done():
		t.Fatalf("scheduler did not start transient plugin: %v", ctx.Err())
	}

	// Confirm both executables still reference their original remote buckets.
	waitForManifestBucket(t, ctx, tb, "spacewave-core", "spacewave-release")
	waitForManifestBucket(t, ctx, tb, "transient-plugin", "transient-release-provider")
	assertOnlyManifestBucket(t, ctx, tb, "spacewave-core", "spacewave-release")
	assertOnlyManifestBucket(t, ctx, tb, "transient-plugin", "transient-release-provider")

	// Admit background materialization and require both refs to become local.
	startupSource.readyCtr.SetValue(bldr_plugin.NewPluginLoadState(
		nil,
		bldr_plugin.InitialCapabilityRegistrationComplete,
	))
	if err := startupGroup.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	waitForManifestBucket(t, ctx, tb, "transient-plugin", worldBucketID(t, ctx, tb))
	waitForManifestBucket(t, ctx, tb, "spacewave-core", worldBucketID(t, ctx, tb))
	// Materializing the same executable locally must preserve the running plugin.
	select {
	case pluginID := <-host.started:
		t.Fatalf("offline copy restarted plugin %q", pluginID)
	default:
	}
}

// buildRemoteManifest stores a minimal executable manifest in an isolated remote bucket.
func buildRemoteManifest(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	le *logrus.Entry,
	bucketID, manifestID string,
) *bldr_manifest.ManifestRef {
	t.Helper()
	// Supply a minimal executable through the ordinary manifest builder.
	distFS := memfs.New()
	f, err := distFS.Create("core.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("export default function main() {}\n")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Commit the fixture manifest into its independently addressed bucket.
	meta := bldr_manifest.NewManifestMeta(manifestID, bldr_manifest.BuildType_RELEASE, "js", 1)
	if _, _, _, err := tb.GetVolume().ApplyBucketConfig(ctx, &bucket.Config{Id: bucketID, Rev: 1}); err != nil {
		t.Fatal(err)
	}
	cursor, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		tb.GetBus(),
		le,
		transform_all.BuildFactorySet(),
		bucketID,
		tb.GetVolume().GetID(),
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer cursor.Release()
	tx, blocks := cursor.BuildTransaction(nil)
	if err := bldr_manifest.CreateManifestWithBilly(ctx, blocks, bldr_manifest.NewManifest(meta, "core.js"), distFS, nil, timestamppb.Now()); err != nil {
		t.Fatal(err)
	}
	root, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	ref := cursor.GetRef().Clone()
	ref.RootRef = root
	return bldr_manifest.NewManifestRef(meta, ref)
}

// releaseFixtureStartupRef exposes controlled startup admission for the fixture.
type releaseFixtureStartupRef struct {
	// readyCtr publishes capability registration independently of plugin execution.
	readyCtr *ccontainer.CContainer[bldr_plugin.PluginLoadState]
}

// GetRunningPluginCtr exposes no running executable for the controlled startup prerequisite.
func (r *releaseFixtureStartupRef) GetRunningPluginCtr() ccontainer.Watchable[bldr_plugin.RunningPlugin] {
	return ccontainer.NewCContainer[bldr_plugin.RunningPlugin](nil)
}

// GetPluginLoadStateCtr exposes the fixture-controlled capability registration state.
func (r *releaseFixtureStartupRef) GetPluginLoadStateCtr() ccontainer.Watchable[bldr_plugin.PluginLoadState] {
	return r.readyCtr
}

// releaseFixtureStartupSource controls the scheduler's background-copy gate.
type releaseFixtureStartupSource struct {
	// ref is shared by all fixture startup references.
	ref *releaseFixtureStartupRef
	// readyCtr publishes the configured startup group's admission state.
	readyCtr *ccontainer.CContainer[bldr_plugin.PluginLoadState]
}

// newReleaseFixtureStartupSource starts the background-copy prerequisite in its pending state.
func newReleaseFixtureStartupSource() *releaseFixtureStartupSource {
	readyCtr := ccontainer.NewCContainer(bldr_plugin.NewPluginLoadState(
		nil,
		bldr_plugin.InitialCapabilityRegistrationPending,
	))
	return &releaseFixtureStartupSource{
		ref:      &releaseFixtureStartupRef{readyCtr: readyCtr},
		readyCtr: readyCtr,
	}
}

// AddPluginReference borrows the shared fixture prerequisite without allocating a new lifetime.
func (s *releaseFixtureStartupSource) AddPluginReference(
	_, _ string,
) (bldr_plugin.RunningPluginRef, func()) {
	return s.ref, func() {}
}

// worldBucketID returns the scheduler world's backing bucket identity.
func worldBucketID(t *testing.T, ctx context.Context, tb *testbed.Testbed) string {
	t.Helper()
	var bucketID string
	if err := tb.GetWorldState().AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		bucketID = cursor.GetOpArgs().GetBucketId()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return bucketID
}

// waitForManifestBucket waits on world sequence changes until the manifest reaches its destination.
func waitForManifestBucket(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	manifestID, bucketID string,
) {
	t.Helper()
	for {
		seqno, err := tb.GetWorldState().GetSeqno(ctx)
		if err != nil {
			t.Fatal(err)
		}
		manifests, errs, err := bldr_manifest_world.CollectManifestsForManifestID(
			ctx,
			tb.GetWorldState(),
			manifestID,
			[]string{"js"},
			tb.GetPluginHostObjKey(),
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(errs) != 0 {
			t.Fatalf("collect %s manifests: %v", manifestID, errs)
		}
		for _, manifest := range manifests {
			if manifest.ManifestRef.GetBucketId() == bucketID {
				return
			}
		}
		if _, err := tb.GetWorldState().WaitSeqno(ctx, seqno+1); err != nil {
			t.Fatal(err)
		}
	}
}

// assertOnlyManifestBucket requires exactly one executable reference in the expected bucket.
func assertOnlyManifestBucket(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	manifestID, bucketID string,
) {
	t.Helper()
	manifests, errs, err := bldr_manifest_world.CollectManifestsForManifestID(
		ctx,
		tb.GetWorldState(),
		manifestID,
		[]string{"js"},
		tb.GetPluginHostObjKey(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 0 {
		t.Fatalf("collect %s manifests: %v", manifestID, errs)
	}
	if len(manifests) != 1 || manifests[0].ManifestRef.GetBucketId() != bucketID {
		t.Fatalf("%s manifest refs = %#v, want only bucket %q", manifestID, manifests, bucketID)
	}
}

// releaseFixturePluginHost observes scheduler admission without running JavaScript.
type releaseFixturePluginHost struct {
	// started records each admitted plugin before waiting for its cancellation.
	started chan string
}

// GetPlatformId selects JavaScript manifests for the fixture host.
func (h *releaseFixturePluginHost) GetPlatformId() string { return "js" }

// Execute keeps the fixture host available until cancellation.
func (h *releaseFixturePluginHost) Execute(ctx context.Context) error {
	<-ctx.Done()
	return context.Canceled
}

// ListPlugins reports no executables before scheduler admission.
func (h *releaseFixturePluginHost) ListPlugins(context.Context) ([]string, error) { return nil, nil }

// ExecutePlugin records admission and holds the executable until cancellation.
func (h *releaseFixturePluginHost) ExecutePlugin(
	ctx context.Context,
	pluginID, instanceKey, manifestRoot, entrypoint string,
	pluginDist, pluginAssets *unixfs.FSHandle,
	hostRpcMux srpc.Mux,
	rpcInit bldr_plugin_host.PluginRpcInitCb,
) error {
	select {
	case h.started <- pluginID:
	default:
	}
	<-ctx.Done()
	return context.Canceled
}

// DeletePlugin accepts removal without maintaining an executable cache.
func (h *releaseFixturePluginHost) DeletePlugin(context.Context, string) error { return nil }
