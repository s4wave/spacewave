package dist_entrypoint

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
)

// DetermineStorageRoot determines the root dir to store data.
func DetermineStorageRoot(projectID string) (string, error) {
	configDir, err := DetermineConfigDir()
	if err != nil {
		return "", err
	}

	outDir := filepath.Join(configDir, projectID)
	return outDir, nil
}

// DetermineConfigDir determines the root config dir.
func DetermineConfigDir() (string, error) {
	switch runtime.GOOS {
	// Darwin has a limit on Unix socket names of 108 characters.
	// Reduce the length of the path by using ~/.aperture
	case "darwin":
		dir := os.Getenv("HOME")
		if dir == "" {
			return "", errors.New("$HOME is not defined")
		}
		return filepath.Join(dir, ".aperture"), nil
	default:
		userConfDir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(userConfDir, "aperture"), nil
	}
}
