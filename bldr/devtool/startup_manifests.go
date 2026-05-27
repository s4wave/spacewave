//go:build !js

package devtool

import bldr_project "github.com/s4wave/spacewave/bldr/project"

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
