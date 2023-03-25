package bldr_manifest_builder

import (
	"path"

	manifest "github.com/aperturerobotics/bldr/manifest"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/pkg/errors"
)

// NewBuilderResult builds the result object.
func NewBuilderResult(
	resultManifest *manifest.Manifest,
	ref *bucket.ObjectRef,
	inputManifest *InputManifest,
) *BuilderResult {
	return &BuilderResult{
		Manifest:      resultManifest,
		ManifestRef:   manifest.NewManifestRef(resultManifest.GetMeta(), ref),
		InputManifest: inputManifest,
	}
}

// Validate validates the BuilderResult.
func (r *BuilderResult) Validate() error {
	if err := r.GetManifest().Validate(); err != nil {
		return errors.Wrap(err, "manifest")
	}
	if err := r.GetManifestRef().Validate(); err != nil {
		return errors.Wrap(err, "manifest_ref")
	}
	if !r.GetManifest().GetMeta().EqualVT(r.GetManifest().GetMeta()) {
		return errors.New("manifest meta must match manifest ref meta")
	}
	if err := r.GetInputManifest().Validate(); err != nil {
		return errors.Wrap(err, "input_manifest")
	}
	return nil
}

// Validate validates the InputManifest
func (m *InputManifest) Validate() error {
	seenPaths := make(map[string]struct{})
	for i, file := range m.GetFiles() {
		filePath := file.GetPath()
		if filePath == "" {
			return errors.Errorf("files[%d]: file path cannot be empty", i)
		}
		cleanedPath := path.Clean(filePath)
		if _, ok := seenPaths[cleanedPath]; ok {
			return errors.Errorf("files[%d]: duplicate file path: %q", i, cleanedPath)
		}
		seenPaths[cleanedPath] = struct{}{}
	}
	return nil
}
