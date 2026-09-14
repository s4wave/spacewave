//go:build !js

package devtool

import (
	"slices"

	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_plugin_compiler_js "github.com/s4wave/spacewave/bldr/plugin/compiler/js"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	web_plugin_compiler "github.com/s4wave/spacewave/bldr/web/plugin/compiler"
)

// StartupManifestPreflight describes one project-owned startup manifest request.
type StartupManifestPreflight struct {
	PluginID    string
	PlatformIDs []string
}

// startupManifestPlatformSelectionPolicies prevents each exact preflight from
// being rebuilt for the other plugin platform after the browser connects.
func startupManifestPlatformSelectionPolicies(
	preflights []StartupManifestPreflight,
) []*plugin_host_scheduler.PlatformSelectionPolicy {
	var platformIDs []string
	for _, preflight := range preflights {
		platformIDs = append(platformIDs, preflight.PlatformIDs...)
	}
	slices.Sort(platformIDs)
	platformIDs = slices.Compact(platformIDs)

	policies := make([]*plugin_host_scheduler.PlatformSelectionPolicy, 0, len(platformIDs))
	for _, platformID := range platformIDs {
		var deniedPluginIDs []string
		for _, preflight := range preflights {
			if !slices.Contains(preflight.PlatformIDs, platformID) {
				deniedPluginIDs = append(deniedPluginIDs, preflight.PluginID)
			}
		}
		if len(deniedPluginIDs) == 0 {
			continue
		}
		slices.Sort(deniedPluginIDs)
		deniedPluginIDs = slices.Compact(deniedPluginIDs)
		policies = append(policies, &plugin_host_scheduler.PlatformSelectionPolicy{
			PlatformId:      platformID,
			DeniedPluginIds: deniedPluginIDs,
		})
	}
	return policies
}

// projectOwnedStartupPlugins returns the plugin ids owned by the project's
// start configuration.
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

// ProjectOwnedStartupManifestPreflight returns the browser-mode startup
// manifest request for one project-owned plugin.
func ProjectOwnedStartupManifestPreflight(
	projectConfig *bldr_project.ProjectConfig,
	pluginID string,
	wasmPlatformID string,
) (StartupManifestPreflight, bool) {
	manifest := projectConfig.GetManifests()[pluginID]
	if manifest == nil {
		return StartupManifestPreflight{}, false
	}
	return StartupManifestPreflight{
		PluginID:    pluginID,
		PlatformIDs: startupManifestPlatformIDs(manifest, "", wasmPlatformID),
	}, true
}

// ProjectOwnedStartupManifestPreflights returns the browser-mode startup
// manifest requests owned by the project.
func ProjectOwnedStartupManifestPreflights(projectConfig *bldr_project.ProjectConfig, wasmPlatformID string) []StartupManifestPreflight {
	pluginIDs := projectOwnedStartupPlugins(projectConfig)
	if len(pluginIDs) == 0 {
		return nil
	}

	preflights := make([]StartupManifestPreflight, 0, len(pluginIDs))
	for _, pluginID := range pluginIDs {
		preflight, ok := ProjectOwnedStartupManifestPreflight(projectConfig, pluginID, wasmPlatformID)
		if ok {
			preflights = append(preflights, preflight)
		}
	}
	return preflights
}

// ProjectOwnedStartupManifestPreflightsForPlatforms narrows known builders to
// the single platform used by the active browser development mode.
func ProjectOwnedStartupManifestPreflightsForPlatforms(
	projectConfig *bldr_project.ProjectConfig,
	goPluginPlatformID,
	wasmPlatformID string,
) []StartupManifestPreflight {
	pluginIDs := projectOwnedStartupPlugins(projectConfig)
	preflights := make([]StartupManifestPreflight, 0, len(pluginIDs))
	for _, pluginID := range pluginIDs {
		manifest := projectConfig.GetManifests()[pluginID]
		if manifest == nil {
			continue
		}
		preflights = append(preflights, StartupManifestPreflight{
			PluginID:    pluginID,
			PlatformIDs: startupManifestPlatformIDs(manifest, goPluginPlatformID, wasmPlatformID),
		})
	}
	return preflights
}

// startupManifestPlatformIDs returns the platform ids a manifest builds
// for, given the resolved wasm platform id.
func startupManifestPlatformIDs(
	manifest *bldr_project.ManifestConfig,
	goPluginPlatformID,
	wasmPlatformID string,
) []string {
	switch manifest.GetBuilder().GetId() {
	case bldr_plugin_compiler_js.ConfigID:
		if goPluginPlatformID == "" && wasmPlatformID != "" && wasmPlatformID != "js" {
			return []string{"js", wasmPlatformID}
		}
		return []string{"js"}
	case bldr_plugin_compiler_go.ConfigID:
		if goPluginPlatformID == "" {
			if wasmPlatformID != "" && wasmPlatformID != "js" {
				return []string{"js", wasmPlatformID}
			}
			return []string{"js"}
		}
		return []string{goPluginPlatformID}
	case web_plugin_compiler.ConfigID:
		return []string{wasmPlatformID}
	default:
		return []string{"js", wasmPlatformID}
	}
}
