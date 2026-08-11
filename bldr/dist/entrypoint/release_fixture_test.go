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
	cdn_bstore_controller "github.com/s4wave/spacewave/core/cdn/bstore/controller"
	cdn_world_controller "github.com/s4wave/spacewave/core/cdn/world/controller"
	block_store_bucket "github.com/s4wave/spacewave/db/block/store/bucket"
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
		"release-world-cdn-store",
		"release-world-cdn-bucket",
	} {
		if launcherConf.GetHostConfigSet()[id] == nil {
			t.Fatalf("launcher host config set missing %q", id)
		}
	}
	if launcherConf.GetConfigSet()["release-world-fetch"] != nil {
		t.Fatal("launcher plugin config set mounts a second release-world-fetch resolver")
	}
	var pluginReleaseWorldConf cdn_world_controller.Config
	pluginReleaseWorld := launcherConf.GetConfigSet()["release-world"]
	if pluginReleaseWorld == nil {
		t.Fatal("launcher config set missing release-world")
	}
	if err := pluginReleaseWorldConf.UnmarshalJSON(pluginReleaseWorld.GetConfig()); err != nil {
		t.Fatalf("decode plugin-bus release-world config: %v", err)
	}
	if pluginReleaseWorldConf.GetCacheBlockStoreId() != "plugin-host" {
		t.Fatalf("plugin-bus release-world cache block store = %q, want plugin-host", pluginReleaseWorldConf.GetCacheBlockStoreId())
	}
	var pluginCdnConf cdn_bstore_controller.Config
	pluginCdnStore := launcherConf.GetConfigSet()["release-world-cdn-store"]
	if pluginCdnStore == nil {
		t.Fatal("launcher config set missing release-world-cdn-store")
	}
	if err := pluginCdnConf.UnmarshalJSON(pluginCdnStore.GetConfig()); err != nil {
		t.Fatalf("decode plugin-bus release-world-cdn-store config: %v", err)
	}
	if pluginCdnConf.GetBlockStoreId() != "spacewave-release-cdn" ||
		pluginCdnConf.GetCacheBlockStoreId() != "plugin-host" ||
		len(pluginCdnConf.GetBucketIds()) != 1 ||
		!slices.Contains(pluginCdnConf.GetBucketIds(), "spacewave-release") {
		t.Fatalf("plugin-bus release CDN config = store %q cache %q buckets %v",
			pluginCdnConf.GetBlockStoreId(), pluginCdnConf.GetCacheBlockStoreId(),
			pluginCdnConf.GetBucketIds())
	}
	var pluginCdnBucketConf block_store_bucket.Config
	pluginCdnBucket := launcherConf.GetConfigSet()["release-world-cdn-bucket"]
	if pluginCdnBucket == nil {
		t.Fatal("launcher config set missing release-world-cdn-bucket")
	}
	if err := pluginCdnBucketConf.UnmarshalJSON(pluginCdnBucket.GetConfig()); err != nil {
		t.Fatalf("decode plugin-bus release-world-cdn-bucket config: %v", err)
	}
	if pluginCdnBucketConf.GetBlockStoreId() != pluginCdnConf.GetBlockStoreId() ||
		pluginCdnBucketConf.GetBucketStoreId() != pluginCdnConf.GetBlockStoreId() ||
		pluginCdnBucketConf.GetBucketConfig().GetId() != "spacewave-release" {
		t.Fatalf("plugin-bus release CDN bucket config = store %q bucket-store %q bucket %q",
			pluginCdnBucketConf.GetBlockStoreId(), pluginCdnBucketConf.GetBucketStoreId(),
			pluginCdnBucketConf.GetBucketConfig().GetId())
	}
	var releaseWorldConf cdn_world_controller.Config
	if err := releaseWorldConf.UnmarshalJSON(launcherConf.GetHostConfigSet()["release-world"].GetConfig()); err != nil {
		t.Fatalf("decode release-world config: %v", err)
	}
	if releaseWorldConf.GetCacheBlockStoreId() != "dist" {
		t.Fatalf("release-world cache block store = %q, want dist", releaseWorldConf.GetCacheBlockStoreId())
	}
	var cdnStoreConf cdn_bstore_controller.Config
	cdnStore := launcherConf.GetHostConfigSet()["release-world-cdn-store"]
	if cdnStore == nil {
		t.Fatal("launcher host config set missing release-world-cdn-store")
	}
	if err := cdnStoreConf.UnmarshalJSON(cdnStore.GetConfig()); err != nil {
		t.Fatalf("decode release-world-cdn-store config: %v", err)
	}
	if cdnStoreConf.GetBlockStoreId() != "spacewave-release-cdn" {
		t.Fatalf("release CDN block store id = %q, want spacewave-release-cdn", cdnStoreConf.GetBlockStoreId())
	}
	if cdnStoreConf.GetSpaceId() != releaseWorldConf.GetSpaceId() ||
		cdnStoreConf.GetCdnBaseUrl() != releaseWorldConf.GetCdnBaseUrl() {
		t.Fatalf("release CDN store origin = (%q, %q), want (%q, %q)",
			cdnStoreConf.GetSpaceId(), cdnStoreConf.GetCdnBaseUrl(),
			releaseWorldConf.GetSpaceId(), releaseWorldConf.GetCdnBaseUrl())
	}
	if cdnStoreConf.GetCacheBlockStoreId() != "dist" {
		t.Fatalf("release CDN cache block store = %q, want dist", cdnStoreConf.GetCacheBlockStoreId())
	}
	if len(cdnStoreConf.GetBucketIds()) != 1 || !slices.Contains(cdnStoreConf.GetBucketIds(), "spacewave-release") {
		t.Fatalf("release CDN bucket ids = %v, want [spacewave-release]", cdnStoreConf.GetBucketIds())
	}
	var cdnBucketConf block_store_bucket.Config
	cdnBucket := launcherConf.GetHostConfigSet()["release-world-cdn-bucket"]
	if cdnBucket == nil {
		t.Fatal("launcher host config set missing release-world-cdn-bucket")
	}
	if err := cdnBucketConf.UnmarshalJSON(cdnBucket.GetConfig()); err != nil {
		t.Fatalf("decode release-world-cdn-bucket config: %v", err)
	}
	if cdnBucketConf.GetBlockStoreId() != cdnStoreConf.GetBlockStoreId() ||
		cdnBucketConf.GetBucketStoreId() != cdnStoreConf.GetBlockStoreId() ||
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
