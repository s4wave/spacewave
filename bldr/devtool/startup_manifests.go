//go:build !js

package devtool

import (
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_plugin_compiler_js "github.com/s4wave/spacewave/bldr/plugin/compiler/js"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	web_plugin_compiler "github.com/s4wave/spacewave/bldr/web/plugin/compiler"
)

// StartupManifestPreflight describes one startup manifest request that should
// be built before opening the browser.
type StartupManifestPreflight struct {
	PluginID    string
	PlatformIDs []string
}

func projectOwnedStartupPlugins(projectConfig *bldr_project.ProjectConfig) []string {
	startPlugins := projectConfig.GetStart().GetPlugins()
	if len(startPlugins) == 0 {
		return nil
	}
	manifests := projectConfig.GetManifests()
	preflightPlugins := make([]string, 0, len(startPlugins))
	for _, pluginID := range startPlugins {
		if _, ok := manifests[pluginID]; ok {
			preflightPlugins = append(preflightPlugins, pluginID)
		}
	}
	return preflightPlugins
}

// ProjectOwnedStartupManifestPreflights returns the browser-mode startup
// manifest requests owned by the project.
func ProjectOwnedStartupManifestPreflights(projectConfig *bldr_project.ProjectConfig, wasmPlatformID string) []StartupManifestPreflight {
	pluginIDs := projectOwnedStartupPlugins(projectConfig)
	if len(pluginIDs) == 0 {
		return nil
	}

	manifests := projectConfig.GetManifests()
	preflights := make([]StartupManifestPreflight, 0, len(pluginIDs))
	for _, pluginID := range pluginIDs {
		manifest := manifests[pluginID]
		preflights = append(preflights, StartupManifestPreflight{
			PluginID:    pluginID,
			PlatformIDs: startupManifestPlatformIDs(pluginID, manifest, wasmPlatformID),
		})
	}
	return preflights
}

func startupManifestPlatformIDs(pluginID string, manifest *bldr_project.ManifestConfig, wasmPlatformID string) []string {
	switch manifest.GetBuilder().GetId() {
	case bldr_plugin_compiler_js.ConfigID:
		return []string{"js"}
	case bldr_plugin_compiler_go.ConfigID, web_plugin_compiler.ConfigID:
		return []string{wasmPlatformID}
	default:
		return []string{"js", wasmPlatformID}
	}
}
