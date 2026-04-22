//go:build !tinygo

package storagepath

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
)

// DetermineConfigDir determines the root config and storage dir.
func DetermineConfigDir(projectID string) (string, error) {
	switch runtime.GOOS {
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
