package dist_entrypoint

import (
	"context"
	"testing"
	"time"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	manifest_fetch_world "github.com/s4wave/spacewave/bldr/manifest/fetch/world"
	cdn_bstore_controller "github.com/s4wave/spacewave/core/cdn/bstore/controller"
	cdn_world_controller "github.com/s4wave/spacewave/core/cdn/world/controller"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	block_store_bucket "github.com/s4wave/spacewave/db/block/store/bucket"
	"github.com/sirupsen/logrus"
)

func TestIsWebDistPlatform(t *testing.T) {
	for _, tt := range []struct {
		platformID string
		want       bool
	}{
		{platformID: "js", want: true},
		{platformID: "web/js/wasm", want: true},
		{platformID: "desktop/js/wasm", want: true},
		{platformID: "desktop/darwin/arm64", want: false},
		{platformID: "linux/amd64", want: false},
	} {
		if got := isWebDistPlatform(tt.platformID); got != tt.want {
			t.Fatalf("isWebDistPlatform(%q) = %v, want %v", tt.platformID, got, tt.want)
		}
	}
}

func TestNewCoreBusResolvesReleaseWorldHostConfigSet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	b, _, err := NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}

	configSet := &configset_proto.ConfigSet{
		Configs: map[string]*configset_proto.ControllerConfig{
			"release-world": {
				Id:     cdn_world_controller.ConfigID,
				Rev:    1,
				Config: []byte(`{"engineId":"spacewave-release-world","spaceId":"01kqjmfxd44r7ggrq78efad3d2","cdnBaseUrl":"https://cdn.spacewave.app"}`),
			},
			"release-world-ops": {
				Id:     space_world_ops.ConfigID,
				Rev:    1,
				Config: []byte(`{"engineId":"spacewave-release-world"}`),
			},
			"release-world-fetch": {
				Id:     manifest_fetch_world.ConfigID,
				Rev:    1,
				Config: []byte(`{"engineId":"spacewave-release-world","objectKeys":["spacewave/release/manifests"]}`),
			},
			"release-world-cdn-store": {
				Id:     cdn_bstore_controller.ConfigID,
				Rev:    1,
				Config: []byte(`{"blockStoreId":"spacewave-release-cdn","spaceId":"01kqjmfxd44r7ggrq78efad3d2","cdnBaseUrl":"https://cdn.spacewave.app","cacheBlockStoreId":"dist","bucketIds":["spacewave-release"]}`),
			},
			"release-world-cdn-bucket": {
				Id:     block_store_bucket.ConfigID,
				Rev:    1,
				Config: []byte(`{"blockStoreId":"spacewave-release-cdn","bucketStoreId":"spacewave-release-cdn","bucketConfig":{"id":"spacewave-release","rev":1}}`),
			},
		},
	}

	resolved, err := configSet.Resolve(ctx, b)
	if err != nil {
		t.Fatalf("resolve Release World host config set: %v", err)
	}
	if got := resolved["release-world"].GetConfig().(*cdn_world_controller.Config).GetEngineId(); got != "spacewave-release-world" {
		t.Fatalf("release-world engine id = %q", got)
	}
	if got := resolved["release-world-ops"].GetConfig().(*space_world_ops.Config).GetEngineId(); got != "spacewave-release-world" {
		t.Fatalf("release-world-ops engine id = %q", got)
	}
	fetchConf := resolved["release-world-fetch"].GetConfig().(*manifest_fetch_world.Config)
	if fetchConf.GetEngineId() != "spacewave-release-world" ||
		len(fetchConf.GetObjectKeys()) != 1 ||
		fetchConf.GetObjectKeys()[0] != "spacewave/release/manifests" {
		t.Fatalf("release-world-fetch config = %#v", fetchConf)
	}
	cdnStoreConf := resolved["release-world-cdn-store"].GetConfig().(*cdn_bstore_controller.Config)
	if cdnStoreConf.GetBlockStoreId() != "spacewave-release-cdn" ||
		cdnStoreConf.GetSpaceId() != "01kqjmfxd44r7ggrq78efad3d2" ||
		cdnStoreConf.GetCdnBaseUrl() != "https://cdn.spacewave.app" ||
		cdnStoreConf.GetCacheBlockStoreId() != "dist" ||
		len(cdnStoreConf.GetBucketIds()) != 1 ||
		cdnStoreConf.GetBucketIds()[0] != "spacewave-release" {
		t.Fatalf("release-world-cdn-store config = %#v", cdnStoreConf)
	}
	cdnBucketConf := resolved["release-world-cdn-bucket"].GetConfig().(*block_store_bucket.Config)
	if cdnBucketConf.GetBlockStoreId() != "spacewave-release-cdn" ||
		cdnBucketConf.GetBucketStoreId() != cdnBucketConf.GetBlockStoreId() ||
		cdnBucketConf.GetBucketConfig().GetId() != "spacewave-release" ||
		cdnBucketConf.GetBucketConfig().GetRev() != 1 {
		t.Fatalf("release-world-cdn-bucket config = %#v", cdnBucketConf)
	}
}
