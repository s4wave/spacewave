//go:build !bldr_startup_trace || tinygo

package bldr_manifest_world

import (
	"context"

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
	return collectStartupManifestEligibilityForManifestID(ctx, ws, manifestID, filterPlatformIDs, objKeys...)
}
