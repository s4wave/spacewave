//go:build bldr_startup_trace && !tinygo

package bldr_manifest_world

import (
	"context"

	startuptrace "github.com/s4wave/spacewave/db/traceutil/startup"
	"github.com/s4wave/spacewave/db/world"
)

// DumpStartupManifestGraphForManifestID builds a non-mutating diagnostic dump
// of the retained manifest graph followed during startup selection.
func DumpStartupManifestGraphForManifestID(
	ctx context.Context,
	ws world.WorldState,
	manifestID string,
	filterPlatformIDs []string,
	objKeys ...string,
) (string, error) {
	traceCtx, task := startuptrace.NewTask(ctx, "bldr/manifest-world/eligibility/graph-dump")
	defer task.End()
	startuptrace.Log(traceCtx, "manifest-id", manifestID)
	dump, err := dumpStartupManifestGraphForManifestID(traceCtx, ws, manifestID, filterPlatformIDs, objKeys...)
	if err != nil {
		startuptrace.Log(traceCtx, "outcome", "error")
		return "", err
	}
	startuptrace.Log(traceCtx, "outcome", "ok")
	return dump, nil
}
