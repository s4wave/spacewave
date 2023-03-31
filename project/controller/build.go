package bldr_project_controller

import (
	"context"
	"sort"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	"golang.org/x/exp/slices"
)

// BuildTargets compiles the given build target(s)
//
// If the targets list is empty, builds all targets.
func (c *Controller) BuildTargets(ctx context.Context, remote string, targets []string, buildType bldr_manifest.BuildType) error {
	projConfig := c.c.GetProjectConfig()
	buildTargets := projConfig.GetBuild()

	// add a remote ref
	remoteRef, err := c.AddRemoteRef(remote)
	if err != nil {
		return err
	}
	defer remoteRef.Release()

	/*
		remoteEngPtr, err := remoteRef.GetResultPromise().Await(ctx)
		if err != nil {
			return err
		}
		remoteEng := *remoteEngPtr
	*/
	bundleObjKey := remoteRef.GetRemoteConfig().GetObjectKey()

	var manifestBuilderConfs []*ManifestBuilderConfig
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}

		buildTarget := buildTargets[target]
		buildTargetManifests := buildTarget.GetManifests()
		platformIDs := slices.Clone(buildTarget.GetPlatformIds())

		// sort & dedupe list of platform ids
		sort.Strings(platformIDs)
		slices.Compact(platformIDs)

		for _, platformID := range platformIDs {
			for _, manifestID := range buildTargetManifests {
				manifestBuilderConfs = append(manifestBuilderConfs, NewManifestBuilderConfig(
					manifestID,
					string(buildType),
					platformID,
					"",
					"",
				))
			}
		}
	}

	_, _, err = c.BuildManifestBundle(ctx, remote, bundleObjKey, manifestBuilderConfs)
	return err
}
