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
	if err := result.ValidateBuildCache(); err != nil {
		return nil, err
	}

	objKey := ManifestBuildResultKey(manifestObjKey)
	obj, objOk, err := ws.GetObject(ctx, objKey)
	if err != nil {
		return nil, err
	}

	if objOk {
		ref, _, err := world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
			bcs.SetBlock(result.CloneVT(), true)
			return nil
		})
		return ref, err
	}

	ref, err := world.AccessObject(ctx, ws.AccessWorldState, nil, func(bcs *block.Cursor) error {
		bcs.SetBlock(result.CloneVT(), true)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if _, err := ws.CreateObject(ctx, objKey, ref); err != nil {
		return nil, err
	}
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
	obj, err := world.MustGetObject(ctx, ws, ManifestBuildResultKey(manifestObjKey))
	if err != nil {
		return nil, nil, err
	}
	var result *bldr_manifest_builder.BuilderResult
	ref, _, err := world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		result, err = bldr_manifest_builder.UnmarshalBuilderResult(ctx, bcs)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	if err := result.ValidateBuildCache(); err != nil {
		return nil, ref, err
	}
	return result, ref, nil
}
