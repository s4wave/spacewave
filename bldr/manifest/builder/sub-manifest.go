package bldr_manifest_builder

import (
	"os"
	"path/filepath"
)

// SubManifestEntrypoint is the no-op entrypoint used by asset-only child Manifests.
const SubManifestEntrypoint = "sub-manifest.mjs"

// WriteSubManifestEntrypoint makes an asset-only child satisfy the Manifest contract.
func WriteSubManifestEntrypoint(distPath string) error {
	return os.WriteFile(filepath.Join(distPath, SubManifestEntrypoint), nil, 0o644)
}
