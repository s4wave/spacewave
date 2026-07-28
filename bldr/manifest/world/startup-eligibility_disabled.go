//go:build !bldr_startup_trace || tinygo

package bldr_manifest_world

import (
	"context"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// CollectStartupManifestEligibilityForManifestID classifies startup manifest
// candidates without mutating the Manifest graph or candidate objects.
func CollectStartupManifestEligibilityForManifestID(
	ctx context.Context,
	ws world.WorldState,
	manifestID string,
	filterPlatformIDs []string,
	objKeys ...string,
) ([]*StartupManifestCandidateEligibility, error) {
	return collectStartupManifestEligibilityForManifestID(ctx, ws, manifestID, filterPlatformIDs, objKeys...)
}
func classifyStartupManifestCandidateEligibility(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	edgeLabel string,
	expectedManifestID string,
	filterPlatformIDs []string,
) (*StartupManifestCandidateEligibility, error) {
	return classifyStartupManifestCandidateEligibilityCore(
		ctx,
		ws,
		objKey,
		edgeLabel,
		expectedManifestID,
		filterPlatformIDs,
	)
}

func getStartupManifestCandidateObjectType(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
) (string, error) {
	return world_types.GetObjectType(ctx, ws, objKey)
}

func lookupManifestRefForStartupEligibility(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
) (*bldr_manifest.ManifestRef, *bucket.ObjectRef, error) {
	return LookupManifestRef(ctx, ws, objKey)
}

func lookupStartupManifestObjectForEligibility(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
) (*bldr_manifest.Manifest, *bucket.ObjectRef, error) {
	return lookupStartupManifestObject(ctx, ws, objKey)
}

func lookupStartupManifestObjectRefLocalForEligibility(
	ctx context.Context,
	ws world.WorldState,
	ref *bucket.ObjectRef,
) (*bldr_manifest.Manifest, error) {
	return lookupStartupManifestObjectRefLocal(ctx, ws, ref)
}

func validateStartupManifest(ctx context.Context, manifest *bldr_manifest.Manifest) error {
	return manifest.Validate()
}

func validateStartupManifestRef(ctx context.Context, manifestRef *bldr_manifest.ManifestRef) error {
	return manifestRef.Validate()
}
