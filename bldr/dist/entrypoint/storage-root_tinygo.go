//go:build tinygo

package dist_entrypoint

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// DetermineConfigDir determines the root config and storage dir.
func DetermineConfigDir() (string, error) {
	switch runtime.GOOS {
	// Darwin has a limit on Unix socket names of 108 characters.
	// Reduce the length of the path by using ~/.aperture
	// Use this for linux as well for consistency.
	case "linux":
		fallthrough
	case "darwin":
		dir := os.Getenv("HOME")
		if dir == "" {
			return "", errors.New("$HOME is not defined")
		}
		return filepath.Join(dir, ".aperture"), nil
	case "js":
		fallthrough
	default:
		return "./.aperture", nil
	}
}
