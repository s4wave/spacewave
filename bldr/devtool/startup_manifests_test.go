//go:build !js

package devtool

import (
	"reflect"
	"testing"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_plugin_compiler_js "github.com/s4wave/spacewave/bldr/plugin/compiler/js"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	web_plugin_compiler "github.com/s4wave/spacewave/bldr/web/plugin/compiler"
)

func TestProjectOwnedStartupPlugins(t *testing.T) {
	projectConfig := &bldr_project.ProjectConfig{
		Start: &bldr_project.StartConfig{
			Plugins: []string{"local-web", "external-web", "local-core"},
		},
		Manifests: map[string]*bldr_project.ManifestConfig{
			"local-core": {},
			"local-web":  {},
		},
	}

	got := projectOwnedStartupPlugins(projectConfig)
	want := []string{"local-web", "local-core"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projectOwnedStartupPlugins() = %v, want %v", got, want)
	}
}

func TestProjectOwnedStartupPluginsEmpty(t *testing.T) {
	if got := projectOwnedStartupPlugins(&bldr_project.ProjectConfig{}); got != nil {
		t.Fatalf("projectOwnedStartupPlugins() = %v, want nil", got)
	}
}

func TestProjectOwnedStartupManifestPreflightsSelectBuilderPlatforms(t *testing.T) {
	projectConfig := &bldr_project.ProjectConfig{
		Start: &bldr_project.StartConfig{
			Plugins: []string{"web", "spacewave-web", "spacewave-app", "spacewave-core", "spacewave-v86", "external"},
		},
		Manifests: map[string]*bldr_project.ManifestConfig{
			"web": {
				Builder: &configset_proto.ControllerConfig{Id: web_plugin_compiler.ConfigID},
			},
			"spacewave-web": {
				Builder: &configset_proto.ControllerConfig{Id: bldr_plugin_compiler_js.ConfigID},
			},
			"spacewave-app": {
				Builder: &configset_proto.ControllerConfig{Id: bldr_plugin_compiler_js.ConfigID},
			},
			"spacewave-core": {
				Builder: &configset_proto.ControllerConfig{Id: bldr_plugin_compiler_go.ConfigID},
			},
			"spacewave-v86": {
				Builder: &configset_proto.ControllerConfig{Id: bldr_plugin_compiler_js.ConfigID},
			},
		},
	}

	got := ProjectOwnedStartupManifestPreflights(projectConfig, "web/js/wasm")
	want := []StartupManifestPreflight{
		{PluginID: "web", PlatformIDs: []string{"web/js/wasm"}},
		{PluginID: "spacewave-web", PlatformIDs: []string{"web/js/wasm"}},
		{PluginID: "spacewave-app", PlatformIDs: []string{"web/js/wasm"}},
		{PluginID: "spacewave-core", PlatformIDs: []string{"web/js/wasm"}},
		{PluginID: "spacewave-v86", PlatformIDs: []string{"web/js/wasm"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ProjectOwnedStartupManifestPreflights() = %#v, want %#v", got, want)
	}
}
