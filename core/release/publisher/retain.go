package publisher

import (
	"context"

	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// DefaultRetain is the revisions of each manifest platform a release keeps,
// enough to roll back by reissue.
const DefaultRetain = 3

// platformKey names one manifest's builds for one platform.
type platformKey struct {
	manifestID, buildType, platformID string
}

// keyOf returns the platform key of meta.
func keyOf(meta *bldr_manifest.ManifestMeta) platformKey {
	return platformKey{meta.GetManifestId(), meta.GetBuildType(), meta.GetPlatformId()}
}

// PruneManifests deletes from the manifest store at manifestKey every
// manifest older than the newest retain revisions of its manifest ID, build
// type and platform. A release published afterwards no longer reaches the
// deleted manifests, so the CDN Space's reclaim drops their content. Returns
// the deleted object keys.
func PruneManifests(ctx context.Context, ws world.WorldState, manifestKey string, retain int) ([]string, error) {
	// Collect every manifest, newest revision first within each manifest ID.
	if retain < 1 {
		return nil, errors.New("retain must keep at least one revision")
	}
	groups, manifestErrors, err := manifest_world.CollectManifests(ctx, ws, nil, manifestKey)
	if err != nil {
		return nil, err
	}
	if len(manifestErrors) != 0 {
		return nil, manifestErrors[0]
	}

	// Unlink each manifest past the newest retain of its platform.
	var pruned []string
	for _, manifests := range groups {
		kept := make(map[platformKey]int)
		for _, manifest := range manifests {
			key := keyOf(manifest.Manifest.GetMeta())
			if kept[key]++; kept[key] <= retain {
				continue
			}
			if err := manifest_world.UnlinkManifest(ctx, ws, manifest.ManifestKey, manifestKey); err != nil {
				return pruned, err
			}
			pruned = append(pruned, manifest.ManifestKey)
		}
	}
	return pruned, nil
}

// ReissueManifest copies the stored manifest meta names to the manifest
// store at manifestKey at a revision above the newest of its platform, and
// returns the copy's meta. The copy shares the original's file trees.
func ReissueManifest(ctx context.Context, ws world.WorldState, manifestKey string, meta *bldr_manifest.ManifestMeta) (*bldr_manifest.ManifestMeta, error) {
	// Find the revision to reissue and the platform's newest.
	groups, _, err := manifest_world.CollectManifests(ctx, ws, []string{meta.GetPlatformId()}, manifestKey)
	if err != nil {
		return nil, err
	}
	var source *bldr_manifest.Manifest
	var newest uint64
	for _, manifest := range groups[meta.GetManifestId()] {
		if keyOf(manifest.Manifest.GetMeta()) != keyOf(meta) {
			continue
		}
		newest = max(newest, manifest.GetRev())
		if manifest.GetRev() == meta.GetRev() {
			source = manifest.Manifest
		}
	}
	if source == nil {
		return nil, errors.Errorf("%s revision %d on %s is not retained", meta.GetManifestId(), meta.GetRev(), meta.GetPlatformId())
	}

	// Write the copy beside the original's file trees.
	reissued := source.CloneVT()
	reissued.Meta.Rev = newest + 1
	ref, err := world.AccessObject(ctx, ws.AccessWorldState, nil, func(cursor *block.Cursor) error {
		cursor.SetBlock(reissued, true)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Store the copy in the manifest store.
	key := bldr_manifest.NewManifestKey(manifestKey, reissued.GetMeta())
	if _, _, err := manifest_world.SetManifest(ctx, ws, "", key, ref); err != nil {
		return nil, err
	}
	if err := ws.SetGraphQuad(ctx, manifest_world.NewManifestQuad(manifestKey, key, reissued.GetMeta().GetManifestId())); err != nil {
		return nil, err
	}
	return reissued.GetMeta(), nil
}
