package bldr_web_bundler_vite_compiler

import (
	"errors"
	"maps"
	"slices"
	"strings"
)

// BuildViteBundleMeta builds the bundle metadata from the bundles.
//
// Deduplicates and combines together multiple entrypoints for the same bundle.
func BuildViteBundleMeta(bundles []*ViteBundleMeta) ([]*ViteBundleMeta, error) {
	// bundleMap is the map of bundle-id to bundle-def
	bundleMap := make(map[string]*ViteBundleMeta)
	for _, bundle := range bundles {
		bundleID := bundle.GetId()
		if bundleID == "" {
			bundleID = "default"
		}

		existingBundle, exists := bundleMap[bundleID]
		if exists {
			// Merge duplicate bundle declarations into the bundle owner. Project
			// config may define the entrypoints while compiler shortcuts add
			// runtime externals for the same id.
			existingBundle.Entrypoints = append(existingBundle.Entrypoints, bundle.GetEntrypoints()...)
			existingBundle.ExternalPkgs = appendMissingStrings(existingBundle.ExternalPkgs, bundle.GetExternalPkgs())
			existingBundle.ViteConfigPaths = appendMissingStrings(existingBundle.ViteConfigPaths, bundle.GetViteConfigPaths())
			existingBundle.DisableProjectConfig = existingBundle.GetDisableProjectConfig() || bundle.GetDisableProjectConfig()
			if existingBundle.GetPublicPath() == "" {
				existingBundle.PublicPath = bundle.GetPublicPath()
			}
		} else {
			bundleMap[bundleID] = bundle
		}
	}

	out := slices.Collect(maps.Values(bundleMap))
	slices.SortFunc(out, func(a, b *ViteBundleMeta) int {
		return strings.Compare(a.GetId(), b.GetId())
	})
	return out, nil
}

func appendMissingStrings(base, extra []string) []string {
	for _, value := range extra {
		if !slices.Contains(base, value) {
			base = append(base, value)
		}
	}
	return base
}

// Validate validates the EsbuildBundleEntrypoint configuration.
func (e *ViteBundleEntrypoint) Validate() error {
	if e.GetInputPath() == "" {
		return errors.New("input_path is required")
	}
	return nil
}
