package bldr_manifest_builder

import (
	manifest "github.com/aperturerobotics/bldr/manifest"
	"github.com/aperturerobotics/hydra/bucket"
)

// BuilderResult is the output of a manifest builder.
type BuilderResult struct {
	// Manifest is the manifest object.
	Manifest *manifest.Manifest
	// ManifestRef is the manifest object ref.
	ManifestRef *manifest.ManifestRef
}

// NewBuilderResult builds the result object.
func NewBuilderResult(resultManifest *manifest.Manifest, ref *bucket.ObjectRef) *BuilderResult {
	return &BuilderResult{
		Manifest:    resultManifest,
		ManifestRef: manifest.NewManifestRef(resultManifest.GetMeta(), ref),
	}
}
