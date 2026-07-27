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

func collectStartupManifestGraph(
	ctx context.Context,
	ws world.WorldState,
	manifestID string,
	objKeys ...string,
) ([]startupManifestGraphEdge, []string, error) {
	traceCtx, task := startuptrace.NewTask(ctx, "bldr/manifest-world/eligibility/graph-walk")
	defer task.End()
	result, err := collectStartupManifestGraphCore(traceCtx, ws, manifestID, objKeys...)
	if err != nil {
		return nil, nil, err
	}
	startuptrace.Logf(
		traceCtx,
		"graph-stats",
		"nodes=%d edges=%d depth=%d",
		result.dequeuedNodes,
		result.edgesFound,
		result.depthReached,
	)
	return result.edges, result.candidates, nil
}

func lookupStartupManifestGraphQuadsBatch(
	ctx context.Context,
	ws world.WorldState,
	objKeys []string,
) ([][]world.GraphQuad, error) {
	traceCtx, task := startuptrace.NewTask(ctx, "bldr/manifest-world/eligibility/graph-batch")
	defer task.End()
	filters := make([]world.GraphQuad, len(objKeys))
	for i, objKey := range objKeys {
		filters[i] = world.NewGraphQuadWithKeys(objKey, PredManifest.String(), "", "")
	}
	return ws.LookupGraphQuadsBatch(
		startuptrace.WithGraphLookupScope(traceCtx),
		filters,
		0,
	)
}
