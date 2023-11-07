package bldr_project_controller

import (
	"context"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
)

// BuildTargets compiles the given build target(s)
//
// If the targets list is empty, builds all targets.
func (c *Controller) BuildTargets(ctx context.Context, remote string, targets []string, buildType bldr_manifest.BuildType) error {
	conf := c.conf.Load()
	projConfig := conf.GetProjectConfig()
	buildTargets := projConfig.GetBuild()

	var manifestBuilderConfs []*ManifestBuilderConfig
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}

		buildTarget := buildTargets[target]
		err := ForManifestSelector(
			buildTarget.GetManifests(),
			buildTarget.GetPlatformIds(),
			func(manifestID, platformID string) (bool, error) {
				manifestBuilderConfs = append(manifestBuilderConfs, NewManifestBuilderConfig(
					manifestID,
					string(buildType),
					platformID,
					remote,
				))
				return true, nil
			},
		)
		if err != nil {
			return err
		}
	}

	_, _, err := c.BuildManifests(ctx, manifestBuilderConfs)
	return err
}
