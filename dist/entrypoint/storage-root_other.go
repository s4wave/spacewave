//go:build !tinygo

package dist_entrypoint

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
)

// DetermineConfigDir determines the root config and storage dir.
func DetermineConfigDir(projectID string) (string, error) {
	switch runtime.GOOS {
	// Darwin has a limit on Unix socket names of 108 characters.
	// Reduce the length of the path by using ~/.aperture
	// Use this for linux as well for consistency.
	case "linux":
		fallthrough
	case "darwin":
		dir := os.Getenv("HOME")
		if dir == "" {
			userConfDir, err := os.UserConfigDir()
			if err != nil || userConfDir == "" {
				return "", errors.New("$HOME is not defined")
			}
			dir = userConfDir
		}
		return filepath.Join(dir, "."+projectID), nil
	case "js":
		return projectID, nil
	default:
		userConfDir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(userConfDir, projectID), nil
	}
}
