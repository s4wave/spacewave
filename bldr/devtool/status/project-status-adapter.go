//go:build !js

package status

import (
	"slices"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	bldr_project_controller "github.com/s4wave/spacewave/bldr/project/controller"
)

// AttachProjectStatus adapts ProjectController config state into Bldr Devtool Status.
func AttachProjectStatus(
	producer *BldrDevtoolStatusProducer,
	controller *bldr_project_controller.Controller,
) {
	controller.SetProjectConfigStatusSink(projectStatusSink{producer: producer})
}

type projectStatusSink struct {
	producer *BldrDevtoolStatusProducer
}

func (s projectStatusSink) SetProjectConfigStatus(projectConfig *bldr_project.ProjectConfig) {
	project := BuildProjectStatus(projectConfig)
	s.producer.UpdateStatus(func(current *BldrDevtoolStatus) *BldrDevtoolStatus {
		return current.WithProject(project)
	})
}

// BuildProjectStatus converts ProjectConfig into the read-only devtool project snapshot.
func BuildProjectStatus(projectConfig *bldr_project.ProjectConfig) BldrDevtoolProjectStatus {
	projectStatus := BldrDevtoolProjectStatus{
		ProjectID:      projectConfig.GetId(),
		StartupPlugins: slices.Clone(projectConfig.GetStart().GetPlugins()),
		WebStartupPath: projectConfig.GetStart().GetLoadWebStartup(),
		ManifestIDs:    sortedMapKeys(projectConfig.GetManifests()),
	}

	buildTargets := projectConfig.GetBuild()
	buildTargetIDs := sortedMapKeys(buildTargets)
	projectStatus.BuildTargets = make([]BldrDevtoolBuildTargetRow, 0, len(buildTargetIDs))
	for _, id := range buildTargetIDs {
		buildConfig := buildTargets[id]
		row := BldrDevtoolBuildTargetRow{
			ID:                  id,
			ManifestIDs:         slices.Clone(buildConfig.GetManifests()),
			ConfiguredTargetIDs: slices.Clone(buildConfig.GetTargets()),
			ExplicitPlatformIDs: slices.Clone(buildConfig.GetPlatformIds()),
			BuildTypes: []string{
				string(bldr_manifest.BuildType_DEV),
				string(bldr_manifest.BuildType_RELEASE),
			},
		}
		resolvedPlatformIDs, err := bldr_project_controller.ResolveBuildConfigPlatformIDs(buildConfig, nil)
		if err != nil {
			row.Error = err.Error()
			projectStatus.BuildTargets = append(projectStatus.BuildTargets, row)
			continue
		}
		row.ResolvedPlatformIDs = resolvedPlatformIDs
		projectStatus.BuildTargets = append(projectStatus.BuildTargets, row)
	}

	return projectStatus
}

func sortedMapKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

var _ bldr_project_controller.ProjectConfigStatusSink = ((*projectStatusSink)(nil))
