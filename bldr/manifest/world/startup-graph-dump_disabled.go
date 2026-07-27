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
