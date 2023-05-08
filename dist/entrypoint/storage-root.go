package dist_entrypoint

import (
	"os"
	"path/filepath"
)

// DetermineStorageRoot determines the root dir to store data.
func DetermineStorageRoot(projectID string) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	outDir := filepath.Join(configDir, "aperture_robotics", projectID)
	return outDir, nil
}
