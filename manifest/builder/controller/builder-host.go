//go:build !js

package bldr_manifest_builder_controller

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_project "github.com/aperturerobotics/bldr/project"
	"github.com/aperturerobotics/util/promise"
	"github.com/pkg/errors"
)

// buildManifestHost is passed to a BuildManifest request to allow requesting additional resources. implements the BuildManifestHost interface.
type buildManifestHost struct {
	c             *Controller
	builderConfig *bldr_manifest_builder.BuilderConfig
	restartFn     func()
}

// newBuildManifestHost builds a new buildManifestHost.
func newBuildManifestHost(c *Controller, builderConfig *bldr_manifest_builder.BuilderConfig, restartFn func()) *buildManifestHost {
	return &buildManifestHost{c: c, builderConfig: builderConfig, restartFn: restartFn}
}

// BuildSubManifest builds a sub-manifest and returns a compilation promise.
//
// subManifestID must be a valid manifest-id.
// in development mode the compiler will watch for changes to the sub-manifest.
//
// once a value is returned from the Promise any change to the sub-manifest
// will restart parent BuildManifest attempt (implementing hot reloading).
func (h *buildManifestHost) BuildSubManifest(
	ctx context.Context,
	subManifestID string,
	subManifestConfig *bldr_project.ManifestConfig,
) (promise.PromiseLike[*bldr_manifest_builder.BuilderResult], error) {
	if ctx.Err() != nil {
		return nil, context.Canceled
	}

	// clone subManifestConfig
	subManifestConfig = subManifestConfig.CloneVT()

	// validate subManifestID is a valid manifest id
	if err := bldr_manifest.ValidateManifestID(subManifestID, false); err != nil {
		return nil, errors.Wrap(err, "invalid sub-manifest id")
	}

	// validate sub-manifest config
	if err := subManifestConfig.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid sub-manifest config")
	}

	// add a reference to the manifest builder for this id
	builderTracker, _ := h.c.subManifestBuilderTrackers.SetKey(subManifestID, true)

	// update the config and get the result promise container, mark the tracker as observed
	return builderTracker.setManifestConfig(subManifestConfig, h.restartFn)
}

// _ is a type assertion
var _ bldr_manifest_builder.BuildManifestHost
