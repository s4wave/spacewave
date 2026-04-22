//go:build tinygo

package storagepath

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// DetermineConfigDir determines the root config and storage dir.
func DetermineConfigDir(projectID string) (string, error) {
	switch runtime.GOOS {
	case "linux":
		fallthrough
	case "darwin":
		dir := os.Getenv("HOME")
		if dir == "" {
			return "", errors.New("$HOME is not defined")
		}
		return filepath.Join(dir, "."+projectID), nil
	case "js":
		return projectID, nil
	default:
		return "./" + projectID, nil
	}
}
