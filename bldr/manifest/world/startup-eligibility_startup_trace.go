//go:build bldr_startup_trace && !tinygo

package bldr_manifest_world

import (
	"context"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/bucket"
	startuptrace "github.com/s4wave/spacewave/db/traceutil/startup"
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
	traceCtx, task := startuptrace.NewTask(ctx, "bldr/manifest-world/eligibility/collect")
	defer task.End()
	startuptrace.Log(traceCtx, "manifest-id", manifestID)
	candidates, err := collectStartupManifestEligibilityForManifestID(traceCtx, ws, manifestID, filterPlatformIDs, objKeys...)
	if err != nil {
		startuptrace.Log(traceCtx, "outcome", "error")
		return nil, err
	}
	startuptrace.Logf(traceCtx, "candidate-count", "%d", len(candidates))
	startuptrace.Log(traceCtx, "outcome", "ok")
	return candidates, nil
}
func classifyStartupManifestCandidateEligibility(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	edgeLabel string,
	expectedManifestID string,
	filterPlatformIDs []string,
) (*StartupManifestCandidateEligibility, error) {
	traceCtx, task := startuptrace.NewTask(ctx, "bldr/manifest-world/eligibility/candidate")
	defer task.End()
	return classifyStartupManifestCandidateEligibilityCore(
		traceCtx,
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
	traceCtx, task := startuptrace.NewTask(ctx, "bldr/manifest-world/eligibility/candidate-type")
	defer task.End()
	return world_types.GetObjectType(traceCtx, ws, objKey)
}

func lookupManifestRefForStartupEligibility(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
) (*bldr_manifest.ManifestRef, *bucket.ObjectRef, error) {
	traceCtx, task := startuptrace.NewTask(ctx, "bldr/manifest-world/eligibility/manifest-ref")
	defer task.End()
	return LookupManifestRef(traceCtx, ws, objKey)
}

func lookupStartupManifestObjectForEligibility(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
) (*bldr_manifest.Manifest, *bucket.ObjectRef, error) {
	traceCtx, task := startuptrace.NewTask(ctx, "bldr/manifest-world/eligibility/manifest")
	defer task.End()
	return lookupStartupManifestObject(traceCtx, ws, objKey)
}

func lookupStartupManifestObjectRefLocalForEligibility(
	ctx context.Context,
	ws world.WorldState,
	ref *bucket.ObjectRef,
) (*bldr_manifest.Manifest, error) {
	traceCtx, task := startuptrace.NewTask(ctx, "bldr/manifest-world/eligibility/manifest")
	defer task.End()
	return lookupStartupManifestObjectRefLocal(traceCtx, ws, ref)
}

func validateStartupManifest(ctx context.Context, manifest *bldr_manifest.Manifest) error {
	_, task := startuptrace.NewTask(ctx, "bldr/manifest-world/eligibility/validate")
	defer task.End()
	return manifest.Validate()
}

func validateStartupManifestRef(ctx context.Context, manifestRef *bldr_manifest.ManifestRef) error {
	_, task := startuptrace.NewTask(ctx, "bldr/manifest-world/eligibility/validate")
	defer task.End()
	return manifestRef.Validate()
}
