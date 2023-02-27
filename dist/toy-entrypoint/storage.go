package main

import (
	"os"
	"path"
)

// DetermineStorageRoot determines the root dir to store data.
func DetermineStorageRoot(appID string) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	outDir := path.Join(configDir, "aperture_robotics", appID)
	return outDir, nil
}
