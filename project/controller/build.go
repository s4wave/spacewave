package bldr_project_controller

import (
	"context"
	"sort"
	"strings"

	bldr_dist "github.com/aperturerobotics/bldr/dist"
	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	"github.com/aperturerobotics/timestamp"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"
)

// BuildTargets compiles the given build target(s)
//
// If the targets list is empty, builds all targets.
func (c *Controller) BuildTargets(ctx context.Context, targets []string, buildType bldr_manifest.BuildType, outputDir string) error {
	projConfig := c.c.GetProjectConfig()
	buildTargets := projConfig.GetBuild()
	// pluginTargets := projConfig.GetPlugin()

	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}

		buildTarget := buildTargets[target]
		buildTargetManifests := buildTarget.GetManifests()
		platformIDs := slices.Clone(buildTarget.GetPlatformIds())

		// sort & dedupe list of ids
		sort.Strings(platformIDs)
		slices.Compact(platformIDs)

		// build list of dist manifests and plugin manifest
		// these will be linked with the output
		var distManifests []*bldr_dist.DistManifest
		var manifestRefs []*bldr_manifest.ManifestRef
		// TODO
		_, _ = distManifests, manifestRefs

		// build the manifests
		var refs []*ManifestBuilderRef
		for _, plugin := range buildTargetManifests {
			// buildTargetPlugin := pluginTargets[plugin]
			for _, pluginPlatformID := range platformIDs {
				meta := bldr_manifest.NewManifestMeta(plugin, buildType, pluginPlatformID, 0)
				refs = append(refs, c.AddManifestBuilderRef(meta))
			}
		}

		// wait for the manifests to finishing building
		for _, ref := range refs {
			result, err := ref.GetResultPromise().Await(ctx)
			if err != nil {
				return err
			}
			// TODO: determine plugin manifest object key
			manifestRefs = append(manifestRefs, result.ManifestRef)
		}

		// now
		now := timestamp.Now()

		// TODO create the manifest bundle
		_ = now
	}

	// TODO
	return errors.New("TODO project controller build targets")
}
