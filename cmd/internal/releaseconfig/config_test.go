package releaseconfig

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	dist_compiler "github.com/s4wave/spacewave/bldr/dist/compiler"
	go_compiler "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	project_starlark "github.com/s4wave/spacewave/bldr/project/starlark"
	cdn_world "github.com/s4wave/spacewave/core/cdn/world/controller"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher/controller"
)

// TestNativeStagingAuthority evaluates the exact export used by both native
// producers and checks all host platforms, the plugin bus and its host bus.
func TestNativeStagingAuthority(t *testing.T) {
	// Copy only producer inputs into an isolated project, then prepare it normally.
	dir := t.TempDir()
	for _, name := range []string{"bldr.star", "release-staging.star"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "..", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("SPACEWAVE_RELEASE_ENV", "staging")
	configPath, err := Prepare(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := project_starlark.Evaluate(filepath.Join(dir, filepath.Dir(configPath), "bldr.star"))
	if err != nil {
		t.Fatal(err)
	}

	// Every native distribution embeds the staging channel and source launcher.
	count := 0
	for name, build := range result.Config.GetBuild() {
		if !strings.HasPrefix(name, "release-desktop-") && !strings.HasPrefix(name, "release-cli-") {
			continue
		}
		count++
		manifestID := "spacewave-dist"
		if strings.HasPrefix(name, "release-cli-") {
			manifestID = "spacewave-cli"
		}
		var conf dist_compiler.Config
		if err := conf.UnmarshalJSON(build.GetManifestOverrides()[manifestID].GetConfig()); err != nil {
			t.Fatal(err)
		}
		if conf.GetChannelKey() != "staging" || len(conf.GetEmbedManifests()) != 1 || conf.GetEmbedManifests()[0].GetManifestId() != "spacewave-launcher" {
			t.Fatalf("%s: wrong channel or embedded launcher: %v", name, &conf)
		}
		if slices.Contains(conf.GetLoadPlugins(), "web") != (manifestID == "spacewave-dist") {
			t.Fatalf("%s: wrong renderer plugin demand", name)
		}
	}
	if count != 12 {
		t.Fatalf("checked %d native builds, want 12", count)
	}

	// Only the staging signer and endpoint may supply this launcher's config.
	var builder go_compiler.Config
	if err := builder.UnmarshalJSON(result.Config.GetManifests()["spacewave-launcher"].GetBuilder().GetConfig()); err != nil {
		t.Fatal(err)
	}
	var launch launcher.Config
	if err := launch.UnmarshalJSON(builder.GetConfigSet()["spacewave-launcher"].GetConfig()); err != nil {
		t.Fatal(err)
	}
	if launch.GetChannelKey() != "staging" || !slices.Equal(launch.GetDistPeerIds(), []string{"12D3KooWJMSrvzA2vZejumpT7o5QZ8BMK4Yb5GYfqiQgXh6d7GoK"}) || len(launch.GetEndpoints()) != 1 || launch.GetEndpoints()[0].GetUrl() != "https://staging.spacewave.app/api/release/config" {
		t.Fatalf("launcher authority = %v", &launch)
	}
	for _, entry := range [][]byte{builder.GetConfigSet()["release-world"].GetConfig(), builder.GetHostConfigSet()["release-world"].GetConfig()} {
		var conf cdn_world.Config
		if err := conf.UnmarshalJSON(entry); err != nil {
			t.Fatal(err)
		}
		if conf.GetSpaceId() != "01kqzhmjchxwkyqxxnjabdnr1t" || conf.GetCdnBaseUrl() != "https://cdn-staging.spacewave.app" {
			t.Fatalf("Release World authority = %v", &conf)
		}
	}
	if builder.GetHostConfigSet()["release-world-cdn-server"] == nil || builder.GetConfigSet()["release-world-cdn-store"].GetId() != "hydra/block/store/rpc" {
		t.Fatal("native launcher must read blocks through its host's Release World")
	}

	// Environment overrides retain the current native core factories and platform config.
	if err := builder.UnmarshalJSON(result.Config.GetManifests()["spacewave-core"].GetBuilder().GetConfig()); err != nil {
		t.Fatal(err)
	}
	var provider provider_spacewave.Config
	if err := provider.UnmarshalJSON(builder.GetConfigSet()["provider-spacewave"].GetConfig()); err != nil {
		t.Fatal(err)
	}
	if provider.GetEndpoint() != "https://staging.spacewave.app" || provider.GetAccountEndpoint() != "https://account-staging.spacewave.app" || provider.GetSigningEnvPrefix() != "spacewave-staging" {
		t.Fatalf("provider authority = %v", &provider)
	}
	if !slices.Contains(builder.GetPlatformTypes()["desktop"].GetGoPkgs(), "./core/resource/desktop/statusprojector") || len(builder.GetPlatformTypes()) == 0 {
		t.Fatal("overlay discarded source-owned native core configuration")
	}
}

// TestPrepareRequiresStagingExport rejects a missing export without production fallback.
func TestPrepareRequiresStagingExport(t *testing.T) {
	t.Setenv("SPACEWAVE_RELEASE_ENV", "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bldr.star"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(dir, "staging"); err == nil {
		t.Fatal("staging accepted a missing release export")
	}
	if path, err := Prepare(dir, "production"); err != nil || path != "bldr.yaml" {
		t.Fatalf("production defaults = %q, %v", path, err)
	}
	if _, err := Prepare(dir, "unknown"); err == nil {
		t.Fatal("unknown environment accepted")
	}
	t.Setenv("SPACEWAVE_RELEASE_ENV", "staging")
	if _, err := Prepare(dir, "production"); err == nil {
		t.Fatal("handoff environment overrode a conflicting producer environment")
	}
}
