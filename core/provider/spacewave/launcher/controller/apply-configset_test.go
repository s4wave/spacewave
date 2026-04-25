package spacewave_launcher_controller

import (
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	manifest_fetch_world "github.com/s4wave/spacewave/bldr/manifest/fetch/world"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	block_store_kvfile_http "github.com/s4wave/spacewave/db/block/store/kvfile/http"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
)

// TestLauncherApplyConfigSet verifies the client-side contract with the
// server-built launcher_config_set: a DistConfig carrying the three expected
// controllers (kvfile/http block store, world/block/engine, manifest/fetch/world)
// passes ConfigSetMap.Validate and decodes back to the same inner fields the
// launcher runtime reads. Mirrors =TestBuildLauncherConfigSet= in
// =repos/spacewave-release/release/launcher-configset_test.go=; the two sides
// must stay in lockstep or the launcher silently fails to mount the release
// world on the client.
func TestLauncherApplyConfigSet(t *testing.T) {
	const (
		rev              uint64 = 7
		worldKVFileURL          = "https://dist.spacewave.app/release/world/7.kvfile"
		engineID                = "spacewave-release-world"
		bucketID                = "spacewave-release"
		objectStoreID           = "spacewave-release"
		httpBlockStoreID        = "spacewave-release-http"
		manifestKey             = "spacewave/release/manifests"
	)

	kvStoreConf := &block_store_kvfile_http.Config{
		BlockStoreId: httpBlockStoreID,
		Url:          worldKVFileURL,
		BucketIds:    []string{bucketID},
	}
	engineConf := &world_block_engine.Config{
		EngineId:             engineID,
		BucketId:             bucketID,
		ObjectStoreId:        objectStoreID,
		DisableApplyWorldOp:  true,
		DisableApplyObjectOp: true,
	}
	fetchConf := &manifest_fetch_world.Config{
		EngineId:            engineID,
		ObjectKeys:          []string{manifestKey},
		OverrideManifestRev: rev,
	}

	entry := func(conf config.Config) *configset_proto.ControllerConfig {
		e, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(rev, conf), false)
		if err != nil {
			t.Fatalf("encode %T: %v", conf, err)
		}
		return e
	}

	launcherSet := map[string]*configset_proto.ControllerConfig{
		"release-world-kvfile-store": entry(kvStoreConf),
		"release-world-engine":       entry(engineConf),
		"release-world-fetch":        entry(fetchConf),
	}

	distConf := &spacewave_launcher.DistConfig{
		ProjectId:         "spacewave",
		Rev:               rev,
		LauncherConfigSet: launcherSet,
	}

	cs := configset_proto.ConfigSetMap(distConf.GetLauncherConfigSet())
	if err := cs.Validate(); err != nil {
		t.Fatalf("ConfigSetMap.Validate: %v", err)
	}

	for key, wantID := range map[string]string{
		"release-world-kvfile-store": "hydra/block/store/kvfile/http",
		"release-world-engine":       "hydra/world/block/engine",
		"release-world-fetch":        "bldr/manifest/fetch/world",
	} {
		got, ok := launcherSet[key]
		if !ok {
			t.Fatalf("missing %q", key)
		}
		if got.GetId() != wantID {
			t.Fatalf("%s id = %q, want %q", key, got.GetId(), wantID)
		}
		if got.GetRev() != rev {
			t.Fatalf("%s rev = %d, want %d", key, got.GetRev(), rev)
		}
	}

	var kv block_store_kvfile_http.Config
	if err := kv.UnmarshalVT(launcherSet["release-world-kvfile-store"].GetConfig()); err != nil {
		t.Fatalf("decode kvfile/http: %v", err)
	}
	if kv.GetUrl() != worldKVFileURL {
		t.Fatalf("kv url = %q, want %q", kv.GetUrl(), worldKVFileURL)
	}
	if kv.GetBlockStoreId() != httpBlockStoreID {
		t.Fatalf("kv block_store_id = %q", kv.GetBlockStoreId())
	}
	if got := kv.GetBucketIds(); len(got) != 1 || got[0] != bucketID {
		t.Fatalf("kv bucket_ids = %v", got)
	}

	var eng world_block_engine.Config
	if err := eng.UnmarshalVT(launcherSet["release-world-engine"].GetConfig()); err != nil {
		t.Fatalf("decode engine: %v", err)
	}
	if eng.GetEngineId() != engineID || eng.GetBucketId() != bucketID || eng.GetObjectStoreId() != objectStoreID {
		t.Fatalf("engine = %+v", &eng)
	}
	if !eng.GetDisableApplyWorldOp() || !eng.GetDisableApplyObjectOp() {
		t.Fatal("engine apply flags not read-only; client would accept remote writes")
	}

	var fetch manifest_fetch_world.Config
	if err := fetch.UnmarshalVT(launcherSet["release-world-fetch"].GetConfig()); err != nil {
		t.Fatalf("decode fetch: %v", err)
	}
	if fetch.GetEngineId() != engineID {
		t.Fatalf("fetch engine_id = %q", fetch.GetEngineId())
	}
	if got := fetch.GetObjectKeys(); len(got) != 1 || got[0] != manifestKey {
		t.Fatalf("fetch object_keys = %v", got)
	}
	if fetch.GetOverrideManifestRev() != rev {
		t.Fatalf("fetch override_manifest_rev = %d, want %d", fetch.GetOverrideManifestRev(), rev)
	}
}
