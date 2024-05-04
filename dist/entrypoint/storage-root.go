package dist_entrypoint

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
