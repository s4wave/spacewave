package dist_entrypoint

import (
	"os"
	"path"
)

// DetermineStorageRoot determines the root dir to store data.
func DetermineStorageRoot(projectID string) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	outDir := path.Join(configDir, "aperture_robotics", projectID)
	return outDir, nil
}
