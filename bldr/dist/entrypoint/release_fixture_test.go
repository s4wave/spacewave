package dist_entrypoint

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/pkg/errors"
	bldr_dist_compiler "github.com/s4wave/spacewave/bldr/dist/compiler"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_fetch_world "github.com/s4wave/spacewave/bldr/manifest/fetch/world"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_project_starlark "github.com/s4wave/spacewave/bldr/project/starlark"
	default_storage "github.com/s4wave/spacewave/bldr/storage/default"
	cdn_world_controller "github.com/s4wave/spacewave/core/cdn/world/controller"
	block_store_bucket "github.com/s4wave/spacewave/db/block/store/bucket"
	block_store_rpc "github.com/s4wave/spacewave/db/block/store/rpc"
	block_store_rpc_server "github.com/s4wave/spacewave/db/block/store/rpc/server"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/sirupsen/logrus"
)

func TestBrowserReleaseLazyPluginFixtureIsNonEmbeddedAndPublished(t *testing.T) {
	result, err := bldr_project_starlark.Evaluate(filepath.Join("..", "..", "..", "bldr.star"))
	if err != nil {
		t.Fatal(err)
	}

	build := result.Config.GetBuild()["release-web-lazy-plugin-fixture"]
	if build == nil {
		t.Fatal("missing release-web-lazy-plugin-fixture build")
	}

	browserOverride := build.GetManifestOverrides()["spacewave-browser"]
	if browserOverride == nil {
		t.Fatal("missing browser fixture manifest override")
	}
	var distConf bldr_dist_compiler.Config
	if err := distConf.UnmarshalJSON(browserOverride.GetConfig()); err != nil {
		t.Fatalf("decode browser fixture dist config: %v", err)
	}
	embeds := distConf.GetEmbedManifests()
	if len(embeds) != 1 || embeds[0].GetManifestId() != "spacewave-launcher" || embeds[0].GetPlatformId() != "js" {
		t.Fatalf("browser release embeds = %#v, want spacewave-launcher@js only", embeds)
	}
	if !slices.Contains(distConf.GetLoadPlugins(), "spacewave-cli-plugin") {
		t.Fatal("lazy fixture does not request terminal plugin through LoadPlugin")
	}

	launcherOverride := build.GetManifestOverrides()["spacewave-launcher"]
	if launcherOverride == nil {
		t.Fatal("missing launcher fixture manifest override")
	}
	var launcherConf bldr_plugin_compiler_go.Config
	if err := launcherConf.UnmarshalJSON(launcherOverride.GetConfig()); err != nil {
		t.Fatalf("decode launcher fixture config: %v", err)
	}
	for _, id := range []string{
		"release-world",
		"release-world-ops",
		"release-world-fetch",
		"release-world-cdn-bucket",
	} {
		if launcherConf.GetHostConfigSet()[id] == nil {
			t.Fatalf("launcher host config set missing %q", id)
		}
	}
	for id := range launcherConf.GetConfigSet() {
		if strings.HasPrefix(id, "release-world") {
			t.Fatalf("browser launcher worker mounts independent Release World config %q", id)
		}
	}
	if launcherConf.GetHostConfigSet()["release-world-cdn-store"] != nil {
		t.Fatal("launcher page host mounts an independent Release World CDN block store")
	}
	var releaseWorldConf cdn_world_controller.Config
	if err := releaseWorldConf.UnmarshalJSON(launcherConf.GetHostConfigSet()["release-world"].GetConfig()); err != nil {
		t.Fatalf("decode release-world config: %v", err)
	}
	if releaseWorldConf.GetCacheBlockStoreId() != "dist" {
		t.Fatalf("release-world cache block store = %q, want dist", releaseWorldConf.GetCacheBlockStoreId())
	}
	var cdnBucketConf block_store_bucket.Config
	if err := cdnBucketConf.UnmarshalJSON(launcherConf.GetHostConfigSet()["release-world-cdn-bucket"].GetConfig()); err != nil {
		t.Fatalf("decode release-world-cdn-bucket config: %v", err)
	}
	if cdnBucketConf.GetBlockStoreId() != cdn_world_controller.ReleaseBlockStoreID ||
		cdnBucketConf.GetBucketStoreId() != cdn_world_controller.ReleaseBlockStoreID ||
		cdnBucketConf.GetBucketConfig().GetId() != "spacewave-release" {
		t.Fatalf("release CDN bucket config = store %q bucket-store %q bucket %q",
			cdnBucketConf.GetBlockStoreId(), cdnBucketConf.GetBucketStoreId(),
			cdnBucketConf.GetBucketConfig().GetId())
	}

	publish := result.Config.GetPublish()["spacewave-release"]
	if publish == nil || !slices.Contains(publish.GetManifests(), "spacewave-cli-plugin") {
		t.Fatal("Release World publication omits the lazy terminal plugin")
	}

	pluginRelease := result.Config.GetBuild()["plugin-release-browser"]
	if pluginRelease == nil {
		t.Fatal("missing plugin-release-browser build")
	}
	for _, manifestID := range []string{"spacewave-core", "spacewave-web", "spacewave-app", "web"} {
		if !slices.Contains(distConf.GetLoadPlugins(), manifestID) {
			t.Fatalf("browser release does not request ordinary startup plugin %s", manifestID)
		}
		if !slices.Contains(publish.GetManifests(), manifestID) ||
			!slices.Contains(pluginRelease.GetManifests(), manifestID) {
			t.Fatalf("Release World omits ordinary startup plugin %s", manifestID)
		}
	}
	for _, embed := range distConf.GetEmbedManifests() {
		manifestID := embed.GetManifestId()
		if !slices.Contains(publish.GetManifests(), manifestID) ||
			!slices.Contains(pluginRelease.GetManifests(), manifestID) {
			continue
		}
		manifest := result.Config.GetManifests()[manifestID]
		if manifest == nil {
			t.Fatalf("missing manifest config for embedded tuple %s@%s", manifestID, embed.GetPlatformId())
		}
		t.Fatalf(
			"release tuple %s@%s#%d has independent embedded and Release World root producers",
			manifestID,
			embed.GetPlatformId(),
			manifest.GetRev(),
		)
	}
}

func TestReleaseLauncherBrowserAndNativeAuthorityComposition(t *testing.T) {
	result, err := bldr_project_starlark.Evaluate(filepath.Join("..", "..", "..", "bldr.star"))
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		build    string
		platform string
		compiler bldr_plugin_compiler_go.GoCompiler
	}{
		{"release-web", "js", bldr_plugin_compiler_go.GoCompiler_GO_COMPILER_GOSCRIPT},
		{"release-web-tinygo", "web", bldr_plugin_compiler_go.GoCompiler_GO_COMPILER_TINYGO},
	} {
		t.Run(tc.build, func(t *testing.T) {
			build := result.Config.GetBuild()[tc.build]
			if tc.build == "release-web-tinygo" && !slices.Equal(build.GetPlatformIds(), []string{"web/js/wasm"}) {
				t.Fatalf("TinyGo release platforms = %v, want web/js/wasm only", build.GetPlatformIds())
			}
			override := build.GetManifestOverrides()["spacewave-launcher"]
			if override == nil {
				t.Fatal("missing browser launcher override")
			}
			var conf bldr_plugin_compiler_go.Config
			if err := conf.UnmarshalJSON(override.GetConfig()); err != nil {
				t.Fatal(err)
			}
			for id := range conf.GetConfigSet() {
				if strings.HasPrefix(id, "release-world") {
					t.Fatalf("browser worker mounts Release World config %q", id)
				}
			}
			for _, id := range []string{"release-world", "release-world-ops", "release-world-fetch", "release-world-cdn-bucket"} {
				if conf.GetHostConfigSet()[id] == nil {
					t.Fatalf("browser host config missing %q", id)
				}
			}
			if conf.GetHostConfigSet()["release-world-cdn-store"] != nil || conf.GetHostConfigSet()["release-world-cdn-server"] != nil {
				t.Fatal("browser host creates a duplicate CDN store or unused RPC bridge")
			}
			platform := conf.GetPlatformTypes()[tc.platform]
			if platform == nil {
				t.Fatalf("launcher platform %q is missing", tc.platform)
			}
			if platform.GetGoCompiler() != tc.compiler {
				t.Fatalf("launcher platform %q compiler = %v, want %v", tc.platform, platform.GetGoCompiler(), tc.compiler)
			}
		})
	}

	launcher := result.Config.GetManifests()["spacewave-launcher"]
	if launcher == nil {
		t.Fatal("missing native launcher manifest")
	}
	var native bldr_plugin_compiler_go.Config
	if err := native.UnmarshalJSON(launcher.GetBuilder().GetConfig()); err != nil {
		t.Fatal(err)
	}
	if native.GetConfigSet()["release-world-fetch"] != nil {
		t.Fatal("native plugin bus mounts a second Release World fetcher")
	}
	var readerConf cdn_world_controller.Config
	if err := readerConf.UnmarshalJSON(native.GetConfigSet()["release-world"].GetConfig()); err != nil {
		t.Fatal(err)
	}
	if readerConf.GetSuppliedBlockStoreId() != cdn_world_controller.ReleaseBlockStoreID || readerConf.GetCacheBlockStoreId() != "" {
		t.Fatalf("native reader supplied=%q cache=%q", readerConf.GetSuppliedBlockStoreId(), readerConf.GetCacheBlockStoreId())
	}
	var clientConf block_store_rpc.Config
	if err := clientConf.UnmarshalJSON(native.GetConfigSet()["release-world-cdn-store"].GetConfig()); err != nil {
		t.Fatal(err)
	}
	if clientConf.GetBlockStoreId() != cdn_world_controller.ReleaseBlockStoreID ||
		clientConf.GetServiceId() != "plugin-host/"+cdn_world_controller.ReleaseBlockStoreID+"/block.rpc.BlockStore" ||
		!clientConf.GetLookupOnStart() {
		t.Fatalf("native RPC client alias=%q service=%q lookupOnStart=%v", clientConf.GetBlockStoreId(), clientConf.GetServiceId(), clientConf.GetLookupOnStart())
	}
	if native.GetHostConfigSet()["release-world-cdn-store"] != nil {
		t.Fatal("native host config creates a block-store alias collision")
	}
	var serverConf block_store_rpc_server.Config
	if err := serverConf.UnmarshalJSON(native.GetHostConfigSet()["release-world-cdn-server"].GetConfig()); err != nil {
		t.Fatal(err)
	}
	if serverConf.GetBlockStoreId() != cdn_world_controller.ReleaseBlockStoreID ||
		serverConf.GetServiceId() != cdn_world_controller.ReleaseBlockStoreID+"/block.rpc.BlockStore" {
		t.Fatalf("native RPC server alias=%q service=%q", serverConf.GetBlockStoreId(), serverConf.GetServiceId())
	}
}

func TestReleaseWorkflowsSeparateEntrypointAndPluginProducers(t *testing.T) {
	entrypointWorkflow, err := os.ReadFile(filepath.Join("..", "..", "..", ".github", "workflows", "entrypoint-release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	entrypointSource := string(entrypointWorkflow)
	forbiddenEntrypointProducers := []string{
		"build-browser-plugin.sh",
		"release-remote-web",
		"release-remote-js",
		"entrypoint-manifest-pack-spacewave-web",
		"entrypoint-manifest-pack-spacewave-app",
	}
	for _, forbidden := range forbiddenEntrypointProducers {
		if strings.Contains(entrypointSource, forbidden) {
			t.Fatalf("entrypoint release retains ordinary plugin producer %q", forbidden)
		}
	}

	desktopGate, err := os.ReadFile(filepath.Join("..", "..", "..", "e2e", "installedapp", "desktop_distribution_gate_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range forbiddenEntrypointProducers {
		if strings.Contains(string(desktopGate), forbidden) {
			t.Fatalf("desktop distribution gate retains ordinary plugin producer %q", forbidden)
		}
	}
	if !strings.Contains(entrypointSource, "SOURCE_DATE_EPOCH=$(git show -s --format=%ct HEAD)") {
		t.Fatal("entrypoint release does not derive SOURCE_DATE_EPOCH from the checked-out commit")
	}

	pluginWorkflow, err := os.ReadFile(filepath.Join("..", "..", "..", ".github", "workflows", "plugin-release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	pluginSource := string(pluginWorkflow)
	for _, required := range []string{
		"build-browser-plugin.sh spacewave-core",
		"build-browser-plugin.sh spacewave-web",
		"build-browser-plugin.sh spacewave-app",
		"build-browser-plugin.sh web",
		"SOURCE_DATE_EPOCH=$(git show -s --format=%ct HEAD)",
	} {
		if !strings.Contains(pluginSource, required) {
			t.Fatalf("plugin release missing ordinary plugin authority %q", required)
		}
	}
}

func TestBrowserReleasePublishedWorldFetchManifestPreflight(t *testing.T) {
	if os.Getenv("E2E_RELEASE_WORLD_PREFLIGHT") != "1" {
		t.Skip("set E2E_RELEASE_WORLD_PREFLIGHT=1 to probe the promoted Release World")
	}
	result, err := bldr_project_starlark.Evaluate(filepath.Join("..", "..", "..", "bldr.star"))
	if err != nil {
		t.Fatal(err)
	}
	build := result.Config.GetBuild()["release-web-lazy-plugin-fixture"]
	if build == nil {
		t.Fatal("missing release-web-lazy-plugin-fixture build")
	}
	launcherOverride := build.GetManifestOverrides()["spacewave-launcher"]
	if launcherOverride == nil {
		t.Fatal("missing launcher fixture manifest override")
	}
	var launcherConf bldr_plugin_compiler_go.Config
	if err := launcherConf.UnmarshalJSON(launcherOverride.GetConfig()); err != nil {
		t.Fatalf("decode launcher fixture config: %v", err)
	}
	hostConfig := launcherConf.GetHostConfigSet()["release-world"]
	if hostConfig == nil {
		t.Fatal("launcher host config set missing release-world")
	}
	var releaseWorldConf cdn_world_controller.Config
	if err := releaseWorldConf.UnmarshalJSON(hostConfig.GetConfig()); err != nil {
		t.Fatalf("decode release-world config: %v", err)
	}
	t.Logf("probing Release World space=%q base=%q", releaseWorldConf.GetSpaceId(), releaseWorldConf.GetCdnBaseUrl())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	b, _, err := NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	cacheBlockStoreID := releaseWorldConf.GetCacheBlockStoreId()
	if cacheBlockStoreID != "dist" {
		t.Fatalf("release-world host cache block store = %q, want dist", cacheBlockStoreID)
	}
	storageID := default_storage.StorageID
	stateRoot := t.TempDir()
	storageCtrl := default_storage.NewController(storageID, b, stateRoot)
	storageCtrlRelease, err := b.AddController(ctx, storageCtrl, nil)
	if err != nil {
		t.Fatalf("add default storage controller: %v", err)
	}
	defer storageCtrlRelease()
	_, _, distVolumeRef, err := loader.WaitExecControllerRunning(
		ctx, b,
		resolver.NewLoadControllerWithConfig(newDistStorageVolumeConfig(storageID, "spacewave")),
		nil,
	)
	if err != nil {
		t.Fatalf("start dist storage volume: %v", err)
	}
	defer distVolumeRef.Release()
	cdnCtrli, _, cdnRef, err := loader.WaitExecControllerRunning(
		ctx, b, resolver.NewLoadControllerWithConfig(&releaseWorldConf), nil,
	)
	if err != nil {
		t.Fatalf("mount Release World: %v", err)
	}
	defer cdnRef.Release()
	cdnCtrl, ok := cdnCtrli.(*cdn_world_controller.Controller)
	if !ok {
		t.Fatalf("Release World controller type = %T", cdnCtrli)
	}
	fetchConf := &manifest_fetch_world.Config{
		EngineId:     releaseWorldConf.GetEngineId(),
		ObjectKeys:   []string{"spacewave/release/manifests"},
		DisableWatch: true,
	}
	_, _, fetchRef, err := loader.WaitExecControllerRunning(
		ctx, b, resolver.NewLoadControllerWithConfig(fetchConf), nil,
	)
	if err != nil {
		t.Fatalf("start Release World FetchManifest resolver: %v", err)
	}
	defer fetchRef.Release()
	engine, err := cdnCtrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatalf("get Release World engine: %v", err)
	}

	for _, tuple := range []struct {
		manifestID string
		platformID string
	}{
		{manifestID: "spacewave-core", platformID: "js"},
		{manifestID: "spacewave-web", platformID: "js"},
		{manifestID: "spacewave-app", platformID: "js"},
		{manifestID: "web", platformID: "web/js/wasm"},
		{manifestID: "spacewave-cli-plugin", platformID: "js"},
	} {
		val, _, valueRef, err := bus.ExecWaitValue[*bldr_manifest.FetchManifestValue](
			ctx, b,
			bldr_manifest.NewFetchManifest(tuple.manifestID, nil, []string{tuple.platformID}, 0),
			bus.ReturnWhenIdle(), nil,
			func(v *bldr_manifest.FetchManifestValue) (bool, error) {
				return len(v.GetManifestRefs()) != 0, nil
			},
		)
		if err != nil {
			t.Fatalf("FetchManifest %s@%s: %v", tuple.manifestID, tuple.platformID, err)
		}
		if valueRef != nil {
			defer valueRef.Release()
		}
		if val == nil {
			t.Fatalf("Release World preflight rejected tuple %s@%s: no provider value", tuple.manifestID, tuple.platformID)
		}
		if len(val.GetManifestRefs()) != 1 {
			t.Fatalf("FetchManifest %s@%s returned %d refs", tuple.manifestID, tuple.platformID, len(val.GetManifestRefs()))
		}
		manifestRef := val.GetManifestRefs()[0]
		if manifestRef.GetMeta().GetManifestId() != tuple.manifestID ||
			manifestRef.GetMeta().GetPlatformId() != tuple.platformID {
			t.Fatalf("unexpected fetched metadata for %s@%s: %#v", tuple.manifestID, tuple.platformID, manifestRef.GetMeta())
		}
		ref := manifestRef.GetManifestRef()
		if ref.GetEmpty() || ref.GetBucketId() == "" {
			t.Fatalf("FetchManifest %s@%s returned non-external ref: %#v", tuple.manifestID, tuple.platformID, ref)
		}
		err = engine.AccessWorldState(ctx, ref, func(cursor *bucket_lookup.Cursor) error {
			_, found, err := cursor.GetBlock(ctx, ref.GetRootRef())
			if err != nil {
				return err
			}
			if !found {
				return errors.New("first manifest root block not found")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("read first %s@%s manifest block: %v", tuple.manifestID, tuple.platformID, err)
		}
		t.Logf("Release World FetchManifest preflight passed tuple=%s@%s root=%s", tuple.manifestID, tuple.platformID, ref.GetRootRef().String())
	}
}
