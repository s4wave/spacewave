package bldr_manifest_builder

import (
	"context"
	"path"

	"github.com/pkg/errors"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
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

// NewBuilderResultBlock constructs a new BuilderResult block.
func NewBuilderResultBlock() block.Block {
	return &BuilderResult{}
}

// UnmarshalBuilderResult unmarshals a BuilderResult block from the cursor.
func UnmarshalBuilderResult(ctx context.Context, bcs *block.Cursor) (*BuilderResult, error) {
	return block.UnmarshalBlock[*BuilderResult](ctx, bcs, NewBuilderResultBlock)
}

// Validate validates the BuilderResult.
func (r *BuilderResult) Validate() error {
	if err := r.GetManifest().Validate(); err != nil {
		return errors.Wrap(err, "manifest")
	}
	if err := r.GetManifestRef().Validate(); err != nil {
		return errors.Wrap(err, "manifest_ref")
	}
	if !r.GetManifest().GetMeta().EqualVT(r.GetManifestRef().GetMeta()) {
		return errors.New("manifest meta must match manifest ref meta")
	}
	if err := r.GetInputManifest().Validate(); err != nil {
		return errors.Wrap(err, "input_manifest")
	}
	for subManifestID, subManifestResult := range r.GetSubManifestResults() {
		if err := manifest.ValidateManifestID(subManifestID, false); err != nil {
			return errors.Wrapf(err, "sub_manifest_results[%q]", subManifestID)
		}
		if subManifestResult == nil {
			return errors.Errorf("sub_manifest_results[%q]: result cannot be nil", subManifestID)
		}
		if err := subManifestResult.Validate(); err != nil {
			return errors.Wrapf(err, "sub_manifest_results[%q]", subManifestID)
		}
	}
	return nil
}

// MarshalBlock marshals the block to binary.
func (r *BuilderResult) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (r *BuilderResult) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
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
		if identity := file.GetIdentity(); identity != nil {
			if identity.GetSizeBytes() == 0 && identity.GetModTimeUnixNano() == 0 && len(identity.GetSha256()) == 0 {
				return errors.Errorf("files[%d]: identity cannot be empty", i)
			}
		}
	}

	seenStartupInputs := make(map[string]struct{})
	for i, input := range m.GetStartupInputs() {
		if input.GetKind() == InputManifest_StartupInputKind_UNKNOWN {
			return errors.Errorf("startup_inputs[%d]: kind cannot be unknown", i)
		}
		if input.GetKey() == "" {
			return errors.Errorf("startup_inputs[%d]: key cannot be empty", i)
		}
		inputKey := input.GetKind().String() + ":" + input.GetKey()
		if _, ok := seenStartupInputs[inputKey]; ok {
			return errors.Errorf("startup_inputs[%d]: duplicate startup input: %q", i, inputKey)
		}
		seenStartupInputs[inputKey] = struct{}{}
	}
	return nil
}

// _ is a type assertion
var _ block.Block = ((*BuilderResult)(nil))
