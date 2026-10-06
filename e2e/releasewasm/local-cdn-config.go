//go:build !js

package releasewasm

import (
	"path/filepath"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
	dist_compiler "github.com/s4wave/spacewave/bldr/dist/compiler"
	compiler "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	project "github.com/s4wave/spacewave/bldr/project"
	starlark "github.com/s4wave/spacewave/bldr/project/starlark"
	cdn_world "github.com/s4wave/spacewave/core/cdn/world/controller"
	launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher/controller"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
)

const (
	localCDNEnv     = "E2E_RELEASE_WASM_LOCAL_CDN"
	localCDNSpaceID = "01kqjmfxd44r7ggrq78efad3d2"
	localCDNState   = ".bldr-startup"
)

// localCDNProject composes the production bootstrap and plugin build targets
// with localhost delivery and the existing public distribution test identity.
func localCDNProject(repoRoot, baseURL string) (*project.ProjectConfig, string, error) {
	// Retain production manifest selection, embedding, and host configuration.
	result, err := starlark.Evaluate(filepath.Join(repoRoot, "bldr.star"))
	if err != nil {
		return nil, "", err
	}
	conf := result.Config
	build := conf.Build["release-web"]
	build.Targets = nil
	build.PlatformIds = []string{"js"}
	var bootstrap compiler.Config
	if err := bootstrap.UnmarshalJSON(build.ManifestOverrides["spacewave-launcher"].Config); err != nil {
		return nil, "", err
	}

	// Read the Release World the distribution mounts before startup.
	var distConf dist_compiler.Config
	if err := distConf.UnmarshalJSON(build.ManifestOverrides["spacewave-browser"].Config); err != nil {
		return nil, "", err
	}

	// Route it to the local CDN origin.
	var worldConf cdn_world.Config
	if err := worldConf.UnmarshalJSON(distConf.HostConfigSet["release-world"].Config); err != nil {
		return nil, "", err
	}
	worldConf.CdnBaseUrl = baseURL + "/cdn"
	worldConf.SpaceId = localCDNSpaceID
	distConf.HostConfigSet["release-world"].Config, err = worldConf.MarshalJSON()
	if err != nil {
		return nil, "", err
	}
	build.ManifestOverrides["spacewave-browser"].Config, err = distConf.MarshalJSON()
	if err != nil {
		return nil, "", err
	}

	// Fetch the signed test distribution through HTTP instead of embedding it.
	var testBootstrap compiler.Config
	if err := testBootstrap.UnmarshalJSON(conf.Build["release-web-e2e-goscript"].ManifestOverrides["spacewave-launcher"].Config); err != nil {
		return nil, "", err
	}
	var dist launcher.Config
	if err := dist.UnmarshalJSON(testBootstrap.ConfigSet["spacewave-launcher"].Config); err != nil {
		return nil, "", err
	}
	packedConfig := dist.InitDistConfig
	if packedConfig == "" {
		return nil, "", errors.New("local CDN requires the signed distribution fixture")
	}

	// Route the launcher to the signed distribution served by the local origin.
	dist.InitDistConfig = ""
	dist.DisableEndpointFetch = false
	dist.Endpoints = []*launcher.HttpEndpoint{{Url: baseURL + "/distribution.packedmsg"}}
	bootstrap.ConfigSet["spacewave-launcher"].Config, err = dist.MarshalJSON()
	if err != nil {
		return nil, "", err
	}
	build.ManifestOverrides["spacewave-launcher"].Config, err = bootstrap.MarshalJSON()
	if err != nil {
		return nil, "", err
	}

	// Build the actual startup closure separately from its two embedded plugins.
	// The local provider keeps foreground storage and RPC without cloud signaling.
	core := conf.Build["release-web-e2e-goscript"].ManifestOverrides["spacewave-core"].CloneVT()
	conf.Build["local-startup-plugins"] = &project.BuildConfig{
		Manifests:         []string{"spacewave-core", "spacewave-web", "spacewave-app"},
		PlatformIds:       []string{"js", "web/js/wasm"},
		ManifestOverrides: map[string]*configset_proto.ControllerConfig{"spacewave-core": core},
	}
	conf.Build["local-startup-web"] = &project.BuildConfig{
		Manifests: []string{"web"}, PlatformIds: []string{"web/js/wasm"},
	}

	// Publish only this closure into a dedicated local World owned by the harness.
	remote := conf.Remotes["spacewave-release"]
	var volume volume_bolt.Config
	if err := volume.UnmarshalJSON(remote.HostConfigSet["release-volume"].Config); err != nil {
		return nil, "", err
	}
	volume.Path = filepath.Join(repoRoot, localCDNState, "publication", "release.bdb")
	remote.HostConfigSet["release-volume"].Config, err = volume.MarshalJSON()
	if err != nil {
		return nil, "", err
	}

	// Select the startup plugin and web manifests for local publication.
	plugins := conf.Publish["spacewave-release"]
	plugins.Manifests = []string{"spacewave-core", "spacewave-web", "spacewave-app"}
	plugins.PlatformIds = []string{"js", "web/js/wasm"}
	web := plugins.CloneVT()
	web.Manifests = []string{"web"}
	web.PlatformIds = []string{"web/js/wasm"}
	conf.Publish["spacewave-release-web"] = web
	return conf, packedConfig, nil
}
