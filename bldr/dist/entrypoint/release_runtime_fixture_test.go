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
	"github.com/go-git/go-billy/v6/memfs"
	bldr_dist_compiler "github.com/s4wave/spacewave/bldr/dist/compiler"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_fetch_world "github.com/s4wave/spacewave/bldr/manifest/fetch/world"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_controller "github.com/s4wave/spacewave/bldr/plugin/host/controller"
	bldr_project_starlark "github.com/s4wave/spacewave/bldr/project/starlark"
	"github.com/s4wave/spacewave/bldr/testbed"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/sirupsen/logrus"
)

func TestLauncherOnlyReleaseConfigMountsWorldAndColdStartsRemoteCore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	result, err := bldr_project_starlark.Evaluate(filepath.Join("..", "..", "..", "bldr.star"))
	if err != nil {
		t.Fatal(err)
	}
	build := result.Config.GetBuild()["release-web-lazy-plugin-fixture"]
	if build == nil {
		t.Fatal("missing launcher-only release fixture")
	}
	browserOverride := build.GetManifestOverrides()["spacewave-browser"]
	if browserOverride == nil {
		t.Fatal("missing browser override")
	}
	var distConf bldr_dist_compiler.Config
	if err := distConf.UnmarshalJSON(browserOverride.GetConfig()); err != nil {
		t.Fatalf("verify launcher-only embedded config: %v", err)
	}
	embeds := distConf.GetEmbedManifests()
	if len(embeds) != 1 || embeds[0].GetManifestId() != "spacewave-launcher" || embeds[0].GetPlatformId() != "js" {
		t.Fatalf("embedded manifests = %#v, want launcher@js only", embeds)
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

	tb, err := testbed.BuildTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	tb.GetStaticResolver().AddFactory(manifest_fetch_world.NewFactory(tb.GetBus()))

	const releaseObjectKey = "spacewave/release/manifests"
	if _, err := bldr_manifest_world.CreateManifestStoreInEngine(ctx, tb.GetWorldEngine(), releaseObjectKey); err != nil {
		t.Fatal(err)
	}
	coreRef := buildRemoteCoreManifest(t, ctx, tb)
	if err := bldr_manifest_world.ExStoreManifestOp(
		ctx,
		tb.GetWorldState(),
		tb.GetVolume().GetPeerID(),
		"release/manifests/spacewave-core/js/1",
		[]string{releaseObjectKey},
		coreRef,
	); err != nil {
		t.Fatal(err)
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
	defer fetchSetRef.Release()

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
	defer fetchRef.Release()
	if len(fetchValue.GetManifestRefs()) != 1 || !fetchValue.GetManifestRefs()[0].EqualVT(coreRef) {
		t.Fatalf("FetchManifest refs = %#v, want remote Core", fetchValue.GetManifestRefs())
	}

	host := &releaseFixturePluginHost{started: make(chan string, 1)}
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
	defer releaseHost()

	_, loadRef, err := tb.GetBus().AddDirective(bldr_plugin.NewLoadPlugin("spacewave-core"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer loadRef.Release()
	select {
	case pluginID := <-host.started:
		if pluginID != "spacewave-core" {
			t.Fatalf("scheduler started %q, want spacewave-core", pluginID)
		}
	case <-ctx.Done():
		t.Fatalf("scheduler did not cold-start remote Core: %v", ctx.Err())
	}
}

func buildRemoteCoreManifest(t *testing.T, ctx context.Context, tb *testbed.Testbed) *bldr_manifest.ManifestRef {
	t.Helper()
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
	meta := bldr_manifest.NewManifestMeta("spacewave-core", bldr_manifest.BuildType_RELEASE, "js", 1)
	var ref *bucket.ObjectRef
	if err := tb.GetWorldEngine().AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		tx, blocks := cursor.BuildTransaction(nil)
		if _, err := bldr_manifest.CreateManifestWithBilly(ctx, blocks, meta, "core.js", distFS, nil, timestamppb.Now()); err != nil {
			return err
		}
		root, _, err := tx.Write(ctx, true)
		if err != nil {
			return err
		}
		ref = cursor.GetRef().Clone()
		ref.RootRef = root
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return bldr_manifest.NewManifestRef(meta, ref)
}

type releaseFixturePluginHost struct {
	started chan string
}

func (h *releaseFixturePluginHost) GetPlatformId() string { return "js" }
func (h *releaseFixturePluginHost) Execute(ctx context.Context) error {
	<-ctx.Done()
	return context.Canceled
}
func (h *releaseFixturePluginHost) ListPlugins(context.Context) ([]string, error) { return nil, nil }
func (h *releaseFixturePluginHost) ExecutePlugin(
	ctx context.Context,
	pluginID, instanceKey, entrypoint string,
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
func (h *releaseFixturePluginHost) DeletePlugin(context.Context, string) error { return nil }
