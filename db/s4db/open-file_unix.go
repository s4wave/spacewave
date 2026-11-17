//go:build darwin || linux

package s4db

import "os"

// openNativeFile opens a native descriptor, permitting failed-build removal.
func openNativeFile(path string, flags int) (*os.File, error) {
	return os.OpenFile(path, flags, 0o600) // #nosec G703 -- the caller chooses the path.
}
