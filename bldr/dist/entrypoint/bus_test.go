package dist_entrypoint

import (
	"context"
	"testing"
	"time"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	manifest_fetch_world "github.com/s4wave/spacewave/bldr/manifest/fetch/world"
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
		t.Fatal(err.Error())
	}

	releaseWorldHostConfigSet := &configset_proto.ConfigSet{
		Configs: map[string]*configset_proto.ControllerConfig{
			"release-world-fetch": {
				Id:     "bldr/manifest/fetch/world",
				Rev:    1,
				Config: []byte(`{"engineId":"spacewave-release-world","objectKeys":["spacewave/release/manifests"],"cdnSpaceId":"01kqjmfxd44r7ggrq78efad3d2","cdnBaseUrl":"https://cdn.spacewave.app","releaseMetadataChannelKey":"stable"}`),
			},
		},
	}

	resolved, err := releaseWorldHostConfigSet.Resolve(ctx, b)
	if err != nil {
		t.Fatalf("resolve Release World host config set: %v", err)
	}

	releaseFetchResolved, ok := resolved["release-world-fetch"]
	if !ok {
		t.Fatal("release-world-fetch config was not resolved")
	}
	releaseFetchConf, ok := releaseFetchResolved.GetConfig().(*manifest_fetch_world.Config)
	if !ok {
		t.Fatalf("release-world-fetch resolved to %T, want *manifest_fetch_world.Config", releaseFetchResolved.GetConfig())
	}
	if releaseFetchConf.GetEngineId() != "spacewave-release-world" ||
		len(releaseFetchConf.GetObjectKeys()) != 1 ||
		releaseFetchConf.GetObjectKeys()[0] != "spacewave/release/manifests" ||
		releaseFetchConf.GetCdnSpaceId() != "01kqjmfxd44r7ggrq78efad3d2" ||
		releaseFetchConf.GetCdnBaseUrl() != "https://cdn.spacewave.app" ||
		releaseFetchConf.GetReleaseMetadataChannelKey() != "stable" {
		t.Fatalf("release-world-fetch config = %#v, want Release World manifest CDN fetch config", releaseFetchConf)
	}
}
