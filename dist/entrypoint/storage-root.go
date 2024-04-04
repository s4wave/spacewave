package dist_entrypoint

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/pkg/errors"
)

// StorageRootEnvVar returns the environment variable for the storage root.
// PROJECT_NAME_DATA_DIR
func StorageRootEnvVar(projectID string) string {
	// Get alphanumeric, dashes, underscores, trim, replace dash to underscore.
	pattern := `[a-zA-Z0-9_-]+`
	re := regexp.MustCompile(pattern)
	matches := re.FindAllString(projectID, -1)
	projectName := strings.Join(matches, "")
	projectName = strings.ReplaceAll(projectName, "-", "_")
	projectName = strings.TrimSpace(projectName)
	return strings.ToUpper(projectName) + "_DATA_DIR"
}

// DetermineStorageRoot determines the root dir to store data.
func DetermineStorageRoot(projectID string) (string, error) {
	envVar := StorageRootEnvVar(projectID)
	envVal := os.Getenv(envVar)
	if envVal != "" {
		return envVal, nil
	}

	configDir, err := DetermineConfigDir()
	if err != nil {
		return "", err
	}

	outDir := filepath.Join(configDir, projectID)
	return outDir, nil
}

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
			userConfDir, err := os.UserConfigDir()
			if err != nil || userConfDir == "" {
				return "", errors.New("$HOME is not defined")
			}
			dir = userConfDir
		}
		return filepath.Join(dir, ".aperture"), nil
	case "js":
		return "/.aperture", nil
	default:
		userConfDir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(userConfDir, "aperture"), nil
	}
}
