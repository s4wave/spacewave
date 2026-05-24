package appbundle

import (
	"path/filepath"
	"strings"
)

// Detect returns whether execPath is inside a macOS .app bundle.
func Detect(execPath string) (bool, string) {
	clean := filepath.Clean(execPath)
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
