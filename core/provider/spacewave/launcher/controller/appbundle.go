package spacewave_launcher_controller

import (
	"path/filepath"
	"strings"
)

// detectAppBundle checks if the given executable path is inside a macOS .app
// bundle (i.e. inside a .app/Contents/MacOS/ directory). Returns whether it
// is a bundle and the root .app directory path.
func detectAppBundle(execPath string) (isBundle bool, bundleRoot string) {
	// Normalize to forward slashes and clean the path.
	clean := filepath.Clean(execPath)
	// Walk up looking for Contents/MacOS pattern inside a .app directory.
	dir := filepath.Dir(clean)
	if filepath.Base(dir) != "MacOS" {
		return false, ""
	}
	contents := filepath.Dir(dir)
	if filepath.Base(contents) != "Contents" {
		return false, ""
	}
	appDir := filepath.Dir(contents)
	if !strings.HasSuffix(appDir, ".app") {
		return false, ""
	}
	return true, appDir
}
