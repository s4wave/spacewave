package bldr_project_controller

import (
	"slices"
	"sort"
)

// ForManifestSelector iterates over all combinations of platform id & manifest id in the given sets.
//
// Callback returns cntu, err
func ForManifestSelector(manifestIDs, platformIDs []string, cb func(manifestID, platformID string) (bool, error)) error {
	// sort & dedupe list of manifests
	manifestIDs = slices.Clone(manifestIDs)
	sort.Strings(manifestIDs)
	manifestIDs = slices.Compact(manifestIDs)

	// sort & dedupe list of platform ids
	platformIDs = slices.Clone(platformIDs)
	sort.Strings(platformIDs)
	platformIDs = slices.Compact(platformIDs)

	for _, platformID := range platformIDs {
		for _, manifestID := range manifestIDs {
			cntu, err := cb(manifestID, platformID)
			if err != nil || !cntu {
				return err
			}
		}
	}

	return nil
}
