package bldr_manifest_world

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// UnlinkMissingStartupManifests deletes the graph edges from rootObjKeys to
// each missing candidate, then deletes the candidate object once no manifest
// edge references it. A missing candidate can never start; a later fetch
// registers the manifest again while a source still holds it. Block bytes
// remain owned by normal World GC. Returns the object keys that were unlinked.
func UnlinkMissingStartupManifests(
	ctx context.Context,
	ws world.WorldState,
	candidates []*StartupManifestCandidateEligibility,
	rootObjKeys ...string,
) ([]string, error) {
	var unlinked []string
	for _, candidate := range candidates {
		if !candidate.Missing {
			continue
		}

		// Delete the edges from each root to the candidate.
		objKey := candidate.ObjectKey
		for _, root := range rootObjKeys {
			quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(root, PredManifest.String(), objKey, ""), 0)
			if err != nil {
				return unlinked, err
			}
			for _, q := range quads {
				if err := ws.DeleteGraphQuad(ctx, q); err != nil {
					return unlinked, err
				}
			}
		}
		unlinked = append(unlinked, objKey)

		// Delete the object once no other manifest edge reaches it.
		inbound, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("", PredManifest.String(), objKey, ""), 1)
		if err != nil {
			return unlinked, err
		}
		if len(inbound) == 0 {
			if _, err := ws.DeleteObject(ctx, objKey); err != nil {
				return unlinked, err
			}
		}
	}
	return unlinked, nil
}
