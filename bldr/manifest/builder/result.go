package bldr_manifest_builder

import (
	"context"
	"path"

	"github.com/pkg/errors"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
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
	if source := r.GetSourceRef(); !source.GetEmpty() {
		if err := source.Validate(); err != nil {
			return errors.Wrap(err, "source_ref")
		}
		if source.GetBucketId() != "" || source.GetRootRef().GetEmpty() {
			return errors.New("source_ref must retain a local source DAG")
		}
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

// ApplyBlockRef updates the retained source DAG or its transform configuration.
func (r *BuilderResult) ApplyBlockRef(id uint32, next *block.BlockRef) error {
	if id != 5 && id != 6 {
		return nil
	}
	if r.SourceRef == nil {
		r.SourceRef = &bucket.ObjectRef{}
	}
	if id == 5 {
		r.SourceRef.RootRef = next
	} else {
		r.SourceRef.TransformConfRef = next
	}
	return nil
}

// GetBlockRefs exposes the local source snapshot to retention and collection.
func (r *BuilderResult) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	return map[uint32]*block.BlockRef{
		5: r.GetSourceRef().GetRootRef(),
		6: r.GetSourceRef().GetTransformConfRef(),
	}, nil
}

// GetBlockRefCtor traverses source directories after the editable tree changes.
func (r *BuilderResult) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 5:
		return unixfs_block.NewFSNodeBlock
	case 6:
		return block_transform.NewTransformConfigBlock
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

// Validate validates the InputManifest.
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

// _ asserts the stored build result and its source retention contract.
var (
	_ block.Block         = (*BuilderResult)(nil)
	_ block.BlockWithRefs = (*BuilderResult)(nil)
)
