package resultworld

import (
	"context"

	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

const (
	// ManifestBuildResultTypeID is the type identifier for Manifest build provenance.
	ManifestBuildResultTypeID = "bldr/manifest-build-result"
)

// ManifestBuildResultKey returns the world object key for a Manifest build result.
func ManifestBuildResultKey(manifestObjKey string) string {
	return manifestObjKey + "/build-result"
}

// SetManifestBuildResult stores build provenance for a Manifest world object.
func SetManifestBuildResult(
	ctx context.Context,
	ws world.WorldState,
	manifestObjKey string,
	result *bldr_manifest_builder.BuilderResult,
) (*bucket.ObjectRef, error) {
	// Validate the build result before storing it.
	if err := result.Validate(); err != nil {
		return nil, err
	}

	// Fetch the existing build result object, if any.
	objKey := ManifestBuildResultKey(manifestObjKey)
	obj, objOk, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, err
	}

	// Overwrite the existing object's block with the new result.
	if objOk {
		ref, _, err := world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
			bcs.SetBlock(result.CloneVT(), true)
			return nil
		})
		return ref, err
	}

	// Create a new World object holding the result block.
	created, ref, err := world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(result.CloneVT(), true)
		return nil
	})
	world.ReleaseObjectState(created)
	if err != nil {
		return nil, err
	}

	// Record the build result type on the object.
	if err := world_types.SetObjectType(ctx, ws, objKey, ManifestBuildResultTypeID); err != nil {
		return nil, err
	}
	return ref, nil
}

// LookupManifestBuildResult looks up world-backed build provenance for a Manifest.
func LookupManifestBuildResult(
	ctx context.Context,
	ws world.WorldState,
	manifestObjKey string,
) (*bldr_manifest_builder.BuilderResult, *bucket.ObjectRef, error) {
	// Fetch the build result object.
	obj, err := world.MustGetObject(ctx, ws, ManifestBuildResultKey(manifestObjKey))
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, nil, err
	}

	// Unmarshal the result block from the object state.
	var result *bldr_manifest_builder.BuilderResult
	ref, _, err := world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		result, err = bldr_manifest_builder.UnmarshalBuilderResult(ctx, bcs)
		return err
	})
	if err != nil {
		return nil, nil, err
	}

	// Validate the decoded result before returning it.
	if err := result.Validate(); err != nil {
		return nil, ref, err
	}
	return result, ref, nil
}
