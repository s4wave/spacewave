//go:build !js

package bldr_manifest_builder_controller

import (
	"context"
	"errors"

	bldr_manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_project "github.com/aperturerobotics/bldr/project"
	"github.com/aperturerobotics/util/promise"
)

// buildManifestHost is passed to a BuildManifest request to allow requesting additional resources. implements the BuildManifestHost interface.
type buildManifestHost struct {
}

// BuildSubManifest builds a sub-manifest and returns a compilation promise.
//
// subManifestID must be a valid manifest-id.
// in development mode the compiler will watch for changes to the sub-manifest.
// once a value is returned from the Promise any change to the sub-manifest will restart parent BuildManifest attempt.
func (h *buildManifestHost) BuildSubManifest(
	ctx context.Context,
	subManifestID string,
	subManifestConfig *bldr_project.ManifestConfig,
) (promise.PromiseLike[*bldr_manifest_builder.BuilderResult], error) {
	// TODO
	return nil, errors.New("TODO implement BuildSubManifest")
}

// _ is a type assertion
var _ bldr_manifest_builder.BuildManifestHost
