//go:build bldr_startup_trace && !tinygo

package bldr_manifest_world

import (
	"context"

	startuptrace "github.com/s4wave/spacewave/db/traceutil/startup"
	"github.com/s4wave/spacewave/db/world"
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
