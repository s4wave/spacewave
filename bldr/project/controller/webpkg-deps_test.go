//go:build !js

package bldr_project_controller

import (
	"slices"
	"testing"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	go_compiler "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	js_compiler "github.com/s4wave/spacewave/bldr/plugin/compiler/js"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	bldr_web_bundler "github.com/s4wave/spacewave/bldr/web/bundler"
	web_runtime_goscript_build "github.com/s4wave/spacewave/bldr/web/runtime/goscript/build"
	"github.com/sirupsen/logrus"
)

func makeJSManifestConfig(t *testing.T, webPkgs []*bldr_web_bundler.WebPkgRefConfig) *bldr_project.ManifestConfig {
	t.Helper()
	conf := &js_compiler.Config{WebPkgs: webPkgs}
	data, err := conf.MarshalVT()
	if err != nil {
		t.Fatalf("marshal JS compiler config: %v", err)
	}
	return &bldr_project.ManifestConfig{
		Builder: &configset_proto.ControllerConfig{
			Id:     js_compiler.ConfigID,
			Config: data,
		},
	}
}

func makeGoManifestConfig(t *testing.T, webPkgs []*bldr_web_bundler.WebPkgRefConfig) *bldr_project.ManifestConfig {
	t.Helper()
	conf := &go_compiler.Config{WebPkgs: webPkgs}
	data, err := conf.MarshalVT()
	if err != nil {
		t.Fatalf("marshal Go compiler config: %v", err)
	}
	return &bldr_project.ManifestConfig{
		Builder: &configset_proto.ControllerConfig{
			Id:     go_compiler.ConfigID,
			Config: data,
		},
	}
}

func TestResolveWebPkgDeps(t *testing.T) {
	manifests := map[string]*bldr_project.ManifestConfig{
		"spacewave-web": makeJSManifestConfig(t, []*bldr_web_bundler.WebPkgRefConfig{
			{Id: "@s4wave/web"},
			{Id: "@fontsource-variable/manrope"},
		}),
		"spacewave-app": makeJSManifestConfig(t, []*bldr_web_bundler.WebPkgRefConfig{
			{Id: "@s4wave/web", Exclude: true},
			{Id: "@fontsource-variable/manrope", Exclude: true},
		}),
		"spacewave-notes": makeJSManifestConfig(t, []*bldr_web_bundler.WebPkgRefConfig{
			{Id: "@s4wave/web", Exclude: true},
		}),
		"spacewave-core": {
			Builder: &configset_proto.ControllerConfig{
				Id: "bldr/plugin/compiler/go",
			},
		},
	}

	deps := resolveWebPkgDeps(logrus.NewEntry(logrus.StandardLogger()), manifests)

	// spacewave-app depends on spacewave-web
	appDeps := deps["spacewave-app"]
	if len(appDeps) != 1 || appDeps[0] != "spacewave-web" {
		t.Fatalf("expected spacewave-app -> [spacewave-web], got %v", appDeps)
	}

	// spacewave-notes depends on spacewave-web
	notesDeps := deps["spacewave-notes"]
	if len(notesDeps) != 1 || notesDeps[0] != "spacewave-web" {
		t.Fatalf("expected spacewave-notes -> [spacewave-web], got %v", notesDeps)
	}

	// spacewave-web and spacewave-core have no deps
	if len(deps["spacewave-web"]) != 0 {
		t.Fatalf("expected no deps for spacewave-web, got %v", deps["spacewave-web"])
	}
	if len(deps["spacewave-core"]) != 0 {
		t.Fatalf("expected no deps for spacewave-core, got %v", deps["spacewave-core"])
	}
}

func TestResolveWebPkgDepsIncludesGoCompilerConfigs(t *testing.T) {
	manifests := map[string]*bldr_project.ManifestConfig{
		"goscript-shared-provider": makeGoManifestConfig(t, []*bldr_web_bundler.WebPkgRefConfig{
			{Id: web_runtime_goscript_build.GoScriptSharedWebPkgID},
		}),
		"goscript-consumer": makeGoManifestConfig(t, []*bldr_web_bundler.WebPkgRefConfig{
			{Id: web_runtime_goscript_build.GoScriptSharedWebPkgID, Exclude: true},
		}),
		"js-provider": makeJSManifestConfig(t, []*bldr_web_bundler.WebPkgRefConfig{
			{Id: "@s4wave/web"},
		}),
		"js-consumer": makeJSManifestConfig(t, []*bldr_web_bundler.WebPkgRefConfig{
			{Id: "@s4wave/web", Exclude: true},
		}),
	}

	deps := resolveWebPkgDeps(logrus.NewEntry(logrus.StandardLogger()), manifests)

	goDeps := deps["goscript-consumer"]
	if len(goDeps) != 1 || goDeps[0] != "goscript-shared-provider" {
		t.Fatalf("expected goscript-consumer -> [goscript-shared-provider], got %v", goDeps)
	}
	jsDeps := deps["js-consumer"]
	if len(jsDeps) != 1 || jsDeps[0] != "js-provider" {
		t.Fatalf("expected js-consumer -> [js-provider], got %v", jsDeps)
	}
	if len(deps["goscript-shared-provider"]) != 0 {
		t.Fatalf("expected no deps for goscript-shared-provider, got %v", deps["goscript-shared-provider"])
	}
}

func TestResolveWebPkgDepsMultipleProviders(t *testing.T) {
	manifests := map[string]*bldr_project.ManifestConfig{
		"provider-a": makeJSManifestConfig(t, []*bldr_web_bundler.WebPkgRefConfig{
			{Id: "@pkg/a"},
		}),
		"provider-b": makeJSManifestConfig(t, []*bldr_web_bundler.WebPkgRefConfig{
			{Id: "@pkg/b"},
		}),
		"consumer": makeJSManifestConfig(t, []*bldr_web_bundler.WebPkgRefConfig{
			{Id: "@pkg/a", Exclude: true},
			{Id: "@pkg/b", Exclude: true},
		}),
	}

	deps := resolveWebPkgDeps(logrus.NewEntry(logrus.StandardLogger()), manifests)
	consumerDeps := deps["consumer"]
	slices.Sort(consumerDeps)
	if len(consumerDeps) != 2 {
		t.Fatalf("expected 2 deps, got %v", consumerDeps)
	}
	if !slices.Contains(consumerDeps, "provider-a") || !slices.Contains(consumerDeps, "provider-b") {
		t.Fatalf("expected [provider-a, provider-b], got %v", consumerDeps)
	}
}
