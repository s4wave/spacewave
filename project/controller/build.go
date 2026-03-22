//go:build !js

package bldr_project_controller

import (
	"context"
	"slices"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	bldr_project "github.com/aperturerobotics/bldr/project"
)

// BuildManifests compiles the given manifest IDs for the native platform.
//
// Returns the manifest refs and object keys for the built manifests.
func (c *Controller) BuildManifests(
	ctx context.Context,
	remote string,
	manifestIDs []string,
	buildType bldr_manifest.BuildType,
) ([]*bldr_manifest.ManifestRef, []string, error) {
	np, err := bldr_platform.ParseNativePlatform("desktop")
	if err != nil {
		return nil, nil, err
	}
	platformID := np.GetPlatformID()

	var confs []*ManifestBuilderConfig
	for _, id := range manifestIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		confs = append(confs, NewManifestBuilderConfig(
			id,
			string(buildType),
			platformID,
			remote,
		))
	}

	return c.BuildManifestBuilderConfigs(ctx, confs)
}

// BuildTargets compiles the given build target(s)
//
// If the targets list is empty, builds all targets.
// If targetsOverride is specified, it overrides the targets field in all build configs.
func (c *Controller) BuildTargets(ctx context.Context, remote string, targets []string, buildType bldr_manifest.BuildType, targetsOverride []string) error {
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
		platformIDs, err := ResolveBuildConfigPlatformIDs(buildTarget, targetsOverride)
		if err != nil {
			return err
		}

		err = ForManifestSelector(
			buildTarget.GetManifests(),
			platformIDs,
			func(manifestID, platformID string) (bool, error) {
				manifestBuilderConfs = append(manifestBuilderConfs, NewManifestBuilderConfigWithTargetPlatforms(
					manifestID,
					string(buildType),
					platformID,
					remote,
					platformIDs,
				))
				return true, nil
			},
		)
		if err != nil {
			return err
		}
	}

	_, _, err := c.BuildManifestBuilderConfigs(ctx, manifestBuilderConfs)
	return err
}

// ResolveBuildConfigPlatformIDs resolves the platform IDs for a BuildConfig.
// If targetsOverride is specified, it takes precedence over the config's targets field.
// Platform IDs from all targets and explicit platform_ids are merged.
func ResolveBuildConfigPlatformIDs(buildConfig *bldr_project.BuildConfig, targetsOverride []string) ([]string, error) {
	var platformIDs []string

	// Use targetsOverride if specified, otherwise use config's targets
	targetIDs := targetsOverride
	if len(targetIDs) == 0 {
		targetIDs = buildConfig.GetTargets()
	}

	// Parse each target and collect platform IDs
	for _, targetID := range targetIDs {
		targetID = strings.TrimSpace(targetID)
		if targetID == "" {
			continue
		}
		target, err := bldr_platform.ParseTarget(targetID)
		if err != nil {
			return nil, err
		}
		platformIDs = append(platformIDs, target.GetPlatformIDs()...)
	}

	// Merge with explicit platform_ids (only if no targets override)
	if len(targetsOverride) == 0 {
		platformIDs = append(platformIDs, buildConfig.GetPlatformIds()...)
	}

	// Deduplicate while preserving order (target platforms first)
	seen := make(map[string]struct{})
	result := make([]string, 0, len(platformIDs))
	for _, id := range platformIDs {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}

	return result, nil
}

// GetBuildConfigTargets returns the parsed Targets for a BuildConfig, if specified.
// Returns nil if no targets are specified.
func GetBuildConfigTargets(buildConfig *bldr_project.BuildConfig) ([]*bldr_platform.Target, error) {
	targetIDs := buildConfig.GetTargets()
	if len(targetIDs) == 0 {
		return nil, nil
	}
	targets := make([]*bldr_platform.Target, 0, len(targetIDs))
	for _, targetID := range targetIDs {
		targetID = strings.TrimSpace(targetID)
		if targetID == "" {
			continue
		}
		target, err := bldr_platform.ParseTarget(targetID)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, nil
}

// MergePlatformIDs merges multiple platform ID lists, preserving order and deduplicating.
func MergePlatformIDs(platformIDLists ...[]string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, list := range platformIDLists {
		for _, id := range list {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				result = append(result, id)
			}
		}
	}
	return result
}

// FilterPlatformIDsByBase filters platform IDs to only those matching the given base platform IDs.
func FilterPlatformIDsByBase(platformIDs []string, basePlatformIDs []string) []string {
	if len(basePlatformIDs) == 0 {
		return nil
	}

	baseSet := make(map[string]struct{}, len(basePlatformIDs))
	for _, bp := range basePlatformIDs {
		baseSet[bp] = struct{}{}
	}

	result := make([]string, 0, len(platformIDs))
	for _, platformID := range platformIDs {
		platform, err := bldr_platform.ParsePlatform(platformID)
		if err != nil {
			continue
		}
		if _, ok := baseSet[platform.GetBasePlatformID()]; ok {
			result = append(result, platformID)
		}
	}
	return slices.Clip(result)
}
