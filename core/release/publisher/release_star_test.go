//go:build !js

package publisher

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	manifest_fetch_world "github.com/s4wave/spacewave/bldr/manifest/fetch/world"
	go_compiler "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	project_starlark "github.com/s4wave/spacewave/bldr/project/starlark"
	cdn_world_controller "github.com/s4wave/spacewave/core/cdn/world/controller"
	launcher_controller "github.com/s4wave/spacewave/core/provider/spacewave/launcher/controller"
	block_store_bucket "github.com/s4wave/spacewave/db/block/store/bucket"
)

// downstreamStar is a downstream project declaring its deployment through the
// vendored release library.
const downstreamStar = `
load(
    "@go/github.com/s4wave/spacewave/core/release/publisher/release.star",
    "release_deployment",
    "release_launcher_controller_config",
    "release_world_config_set",
    "release_world_reader_config_set",
    "release_world_serve_config_set",
)

DEPLOYMENT = release_deployment(
    project_id = "example",
    space_id = "01kqzhmjchxwkyqxxnjabdnr1t",
    signer_peer_id = "12D3KooWJPNip1SbsUG7SjteoegGjfq22D4WThuctPvCwJzHesD5",
    selector_url = "https://example.test/api/release/config",
    object_key = "example/release/manifests",
    cdn_base_url = "https://cdn.example.test",
    root_pointer_base_url = "https://example.test/cdn",
)

config_set = {"example-launcher": config_entry(
    "spacewave/launcher/controller", 1,
    release_launcher_controller_config(DEPLOYMENT),
)}
config_set.update(release_world_reader_config_set(DEPLOYMENT))
host_config_set = release_world_config_set(DEPLOYMENT)
host_config_set.update(release_world_serve_config_set(DEPLOYMENT))

manifest("example-launcher", builder = "bldr/plugin/compiler/go", rev = 1, config = {
    "goPkgs": ["github.com/s4wave/spacewave/core/provider/spacewave/launcher/controller"],
    "configSet": config_set,
    "hostConfigSet": host_config_set,
})
build("release-linux-amd64", manifests = ["example-launcher"], platform_ids = ["desktop/linux/amd64"])
project(id = "example")
`

// TestReleaseStarMatchesDeployment evaluates a project outside the repository
// that loads the release library from its vendor directory, and checks the
// release World host configs equal Deployment.HostConfigSet's and the launcher
// selects the deployment's channel.
func TestReleaseStarMatchesDeployment(t *testing.T) {
	// Vendor the library into a project of its own.
	dir := t.TempDir()
	library, err := os.ReadFile("release.star")
	if err != nil {
		t.Fatal(err)
	}
	vendored := filepath.Join(dir, "vendor", "github.com", "s4wave", "spacewave", "core", "release", "publisher")
	if err := os.MkdirAll(vendored, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vendored, "release.star"), library, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bldr.star"), []byte(downstreamStar), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := project_starlark.Evaluate(filepath.Join(dir, "bldr.star"))
	if err != nil {
		t.Fatal(err)
	}

	// The release target builds the launcher.
	if got := result.Config.GetBuild()["release-linux-amd64"].GetManifests(); !slices.Equal(got, []string{"example-launcher"}) {
		t.Fatalf("release target manifests: %v", got)
	}
	var builder go_compiler.Config
	if err := builder.UnmarshalJSON(result.Config.GetManifests()["example-launcher"].GetBuilder().GetConfig()); err != nil {
		t.Fatal(err)
	}

	// The host config holds Deployment.HostConfigSet, and the server for the
	// reader on the plugin bus.
	d := Deployment{
		ProjectID:          "example",
		SpaceID:            "01kqzhmjchxwkyqxxnjabdnr1t",
		CdnBaseURL:         "https://cdn.example.test",
		RootPointerBaseURL: "https://example.test/cdn",
		ManifestKey:        "example/release/manifests",
	}
	want, err := d.HostConfigSet()
	if err != nil {
		t.Fatal(err)
	}
	host := builder.GetHostConfigSet()
	if len(host) != len(want)+1 || host["release-world-cdn-server"] == nil {
		t.Fatalf("host config keys: %v", slices.Sorted(maps.Keys(host)))
	}
	for key, conf := range want {
		if !sameControllerConfig(t, conf, host[key]) {
			t.Fatalf("%s: library config differs from Deployment.HostConfigSet", key)
		}
	}

	// The plugin bus reads the host's store and mounts the same Space.
	var reader cdn_world_controller.Config
	if err := reader.UnmarshalJSON(builder.GetConfigSet()["release-world"].GetConfig()); err != nil {
		t.Fatal(err)
	}
	if reader.GetSuppliedBlockStoreId() != d.SpaceID || reader.GetRootPointerBaseUrl() != d.RootPointerBaseURL || reader.GetEngineId() != "example-release-world" {
		t.Fatalf("reader mount: %v", &reader)
	}

	// The launcher verifies the deployment's signer and fetches its selector.
	var launch launcher_controller.Config
	if err := launch.UnmarshalJSON(builder.GetConfigSet()["example-launcher"].GetConfig()); err != nil {
		t.Fatal(err)
	}
	if launch.GetProjectId() != "example" || !slices.Equal(launch.GetDistPeerIds(), []string{"12D3KooWJPNip1SbsUG7SjteoegGjfq22D4WThuctPvCwJzHesD5"}) || len(launch.GetEndpoints()) != 1 || launch.GetEndpoints()[0].GetUrl() != "https://example.test/api/release/config" {
		t.Fatalf("launcher: %v", &launch)
	}
}

// sameControllerConfig reports whether the binary config want and the JSON
// config got decode to the same controller config.
func sameControllerConfig(t *testing.T, want, got *configset_proto.ControllerConfig) bool {
	t.Helper()
	if got == nil || got.GetId() != want.GetId() || got.GetRev() != want.GetRev() {
		return false
	}
	switch want.GetId() {
	case cdn_world_controller.ConfigID:
		var a, b cdn_world_controller.Config
		return decodeBoth(t, &a, &b, want, got) && a.EqualVT(&b)
	case block_store_bucket.ConfigID:
		var a, b block_store_bucket.Config
		return decodeBoth(t, &a, &b, want, got) && a.EqualVT(&b)
	case manifest_fetch_world.ConfigID:
		var a, b manifest_fetch_world.Config
		return decodeBoth(t, &a, &b, want, got) && a.EqualVT(&b)
	}
	t.Fatalf("unexpected controller config %s", want.GetId())
	return false
}

// controllerConfigMsg decodes a controller config from binary and JSON.
type controllerConfigMsg interface {
	UnmarshalVT([]byte) error
	UnmarshalJSON([]byte) error
}

// decodeBoth decodes want's binary config into a and got's JSON config into b.
func decodeBoth(t *testing.T, a, b controllerConfigMsg, want, got *configset_proto.ControllerConfig) bool {
	t.Helper()
	if err := a.UnmarshalVT(want.GetConfig()); err != nil {
		t.Fatal(err)
	}
	if err := b.UnmarshalJSON(got.GetConfig()); err != nil {
		t.Fatal(err)
	}
	return true
}
