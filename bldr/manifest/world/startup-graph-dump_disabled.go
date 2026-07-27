//go:build !bldr_startup_trace || tinygo

package bldr_manifest_world

import (
	"context"

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
	return dumpStartupManifestGraphForManifestID(ctx, ws, manifestID, filterPlatformIDs, objKeys...)
}

func collectStartupManifestGraph(
	ctx context.Context,
	ws world.WorldState,
	manifestID string,
	objKeys ...string,
) ([]startupManifestGraphEdge, []string, error) {
	result, err := collectStartupManifestGraphCore(ctx, ws, manifestID, objKeys...)
	if err != nil {
		return nil, nil, err
	}
	return result.edges, result.candidates, nil
}

func lookupStartupManifestGraphQuadsBatch(
	ctx context.Context,
	ws world.WorldState,
	objKeys []string,
) ([][]world.GraphQuad, error) {
	filters := make([]world.GraphQuad, len(objKeys))
	for i, objKey := range objKeys {
		filters[i] = world.NewGraphQuadWithKeys(objKey, PredManifest.String(), "", "")
	}
	return ws.LookupGraphQuadsBatch(ctx, filters, 0)
}
