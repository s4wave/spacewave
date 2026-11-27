//go:build !js

package bldr_manifest_builder_controller

import (
	"context"

	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
)

// buildManifestHost is passed to a BuildManifest request to allow requesting additional resources. implements the BuildManifestHost interface.
type buildManifestHost struct {
	trackers  *keyed.Keyed[string, *subManifestBuilderTracker]
	restartFn func(string)
}

// newBuildManifestHost builds a buildManifestHost that adds sub-manifest
// builds to trackers and binds them to restartFn.
func newBuildManifestHost(trackers *keyed.Keyed[string, *subManifestBuilderTracker], restartFn func(string)) *buildManifestHost {
	return &buildManifestHost{trackers: trackers, restartFn: restartFn}
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
	// Reject the call once the controller context is canceled.
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
	builderTracker, _ := h.trackers.SetKey(subManifestID, true)

	// update the config and get the result promise container, mark the tracker as observed
	return builderTracker.setManifestConfig(subManifestConfig, h.restartFn)
}

// _ is a type assertion
var _ bldr_manifest_builder.BuildManifestHost = (*buildManifestHost)(nil)
