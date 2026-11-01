//go:build !js

package devtool_store

import (
	"bytes"
	"cmp"
	"context"
	"slices"

	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// CollectLatestManifestSet returns one latest manifest reference per platform.
//
// It rejects unavailable or invalid candidates and ambiguous equal-revision
// references instead of selecting one by traversal order. Returned references
// are sorted by manifest ID, descending revision, platform ID, and reference.
func CollectLatestManifestSet(
	ctx context.Context,
	ws world.WorldState,
	manifestID string,
) ([]*bldr_manifest.ManifestRef, error) {
	// Validate the manifest ID and collect its candidates.
	if err := bldr_manifest.ValidateManifestID(manifestID, false); err != nil {
		return nil, err
	}
	manifests, manifestErrs, err := bldr_manifest_world.CollectManifests(ctx, ws, nil, PluginHostObjectKey)
	if err != nil {
		return nil, err
	}
	if len(manifestErrs) != 0 {
		return nil, errors.Wrap(manifestErrs[0], "collect manifest set")
	}

	// Select the manifest's candidate references.
	candidates := manifests[manifestID]
	if len(candidates) == 0 {
		return nil, errors.Errorf("manifest %q not found", manifestID)
	}

	// Validate each candidate and keep the latest per platform.
	selected := make(map[string]*bldr_manifest.ManifestRef)
	for _, candidate := range candidates {
		meta := candidate.Manifest.GetMeta()
		if err := candidate.Manifest.Validate(); err != nil {
			return nil, errors.Wrapf(err, "manifest %s", candidate.ManifestKey)
		}
		if _, err := bldr_platform.ParsePlatform(meta.GetPlatformId()); err != nil {
			return nil, errors.Wrapf(err, "manifest %s platform", candidate.ManifestKey)
		}
		canonicalRef, err := bldr_manifest_world.CanonicalizeManifestObjectRef(ctx, ws.AccessWorldState, candidate.ManifestRef)
		if err != nil {
			return nil, errors.Wrapf(err, "manifest %s reference", candidate.ManifestKey)
		}
		if canonicalRef.GetRootRef().GetEmpty() {
			return nil, errors.Errorf("manifest %s has empty root reference", candidate.ManifestKey)
		}
		ref := bldr_manifest.NewManifestRef(meta, canonicalRef)
		if err := ref.Validate(); err != nil {
			return nil, errors.Wrapf(err, "manifest %s", candidate.ManifestKey)
		}
		platformID := meta.GetPlatformId()
		current := selected[platformID]
		if current == nil || meta.GetRev() > current.GetMeta().GetRev() {
			selected[platformID] = ref
			continue
		}
		if meta.GetRev() != current.GetMeta().GetRev() {
			continue
		}
		if !meta.EqualVT(current.GetMeta()) || !canonicalRef.EqualsRef(current.GetManifestRef()) {
			return nil, errors.Errorf("manifest %q platform %q revision %d has ambiguous references", manifestID, platformID, meta.GetRev())
		}
	}
	if len(selected) == 0 {
		return nil, errors.Errorf("manifest %q has no valid platforms", manifestID)
	}
	result := make([]*bldr_manifest.ManifestRef, 0, len(selected))
	for _, ref := range selected {
		result = append(result, ref)
	}
	slices.SortStableFunc(result, func(a, b *bldr_manifest.ManifestRef) int {
		// Sort the selected references by ID, revision, platform, and ref.
		if c := cmp.Compare(a.GetMeta().GetManifestId(), b.GetMeta().GetManifestId()); c != 0 {
			return c
		}
		if c := cmp.Compare(b.GetMeta().GetRev(), a.GetMeta().GetRev()); c != 0 {
			return c
		}
		if c := cmp.Compare(a.GetMeta().GetPlatformId(), b.GetMeta().GetPlatformId()); c != 0 {
			return c
		}
		aBytes, _ := a.GetManifestRef().MarshalVT()
		bBytes, _ := b.GetManifestRef().MarshalVT()
		return bytes.Compare(aBytes, bBytes)
	})
	return result, nil
}

// LookupManifestSet opens the devtool World on vol and collects one latest
// manifest reference for every valid platform.
func LookupManifestSet(
	ctx context.Context,
	le *logrus.Entry,
	vol volume.Volume,
	manifestID string,
) ([]*bldr_manifest.ManifestRef, error) {
	// Open the devtool World engine and collect the manifest set.
	eng, err := OpenWorld(ctx, le, vol, false)
	if err != nil {
		return nil, err
	}
	defer eng.Close()

	// Collect the latest manifest set from the engine World state.
	ws := world.NewEngineWorldState(eng, false)
	refs, err := CollectLatestManifestSet(ctx, ws, manifestID)
	if err != nil {
		return nil, errors.Wrap(err, "collect manifest set")
	}
	return refs, nil
}

// LookupManifests opens the devtool World on vol and returns the latest
// revision of the manifests with manifestID.
func LookupManifests(
	ctx context.Context,
	le *logrus.Entry,
	vol volume.Volume,
	manifestID string,
) ([]*bldr_manifest_world.CollectedManifest, error) {
	// Open the devtool World engine.
	eng, err := OpenWorld(ctx, le, vol, false)
	if err != nil {
		return nil, err
	}
	defer eng.Close()

	// Collect the manifests for the plugin host object.
	ws := world.NewEngineWorldState(eng, false)
	manifests, _, err := bldr_manifest_world.CollectManifests(ctx, ws, nil, PluginHostObjectKey)
	if err != nil {
		return nil, errors.Wrap(err, "collect manifests")
	}

	// Reject an unknown manifest ID.
	list, ok := manifests[manifestID]
	if !ok || len(list) == 0 {
		available := make([]string, 0, len(manifests))
		for id := range manifests {
			available = append(available, id)
		}
		return nil, errors.Errorf("manifest %q not found (available: %v)", manifestID, available)
	}

	// Filter the manifest list to the latest revision.
	list = bldr_manifest_world.FilterCollectedManifestsByLatestRev(list)
	return list, nil
}
