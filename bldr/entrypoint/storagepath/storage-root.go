package storagepath

import (
	"os"
	"regexp"
	"strings"
)

// StorageRootEnvVar returns the environment variable for the storage root.
func StorageRootEnvVar(projectID string) string {
	return projectIDPrefix(projectID) + "_DATA_DIR"
}

// LogLevelEnvVar returns the environment variable that overrides the log
// level for the given project (e.g. "spacewave" -> "SPACEWAVE_LOG_LEVEL").
func LogLevelEnvVar(projectID string) string {
	return projectIDPrefix(projectID) + "_LOG_LEVEL"
}

// LogRetentionDaysEnvVar returns the environment variable that overrides
// the on-disk log retention duration (in days) for the given project.
func LogRetentionDaysEnvVar(projectID string) string {
	return projectIDPrefix(projectID) + "_LOG_RETENTION_DAYS"
}

// projectIDPrefix sanitizes projectID into the upper-cased prefix used by
// project-scoped environment variables (e.g. "spacewave" -> "SPACEWAVE").
func projectIDPrefix(projectID string) string {
	pattern := `[a-zA-Z0-9_-]+`
	re := regexp.MustCompile(pattern)
	matches := re.FindAllString(projectID, -1)
	projectName := strings.Join(matches, "")
	projectName = strings.ReplaceAll(projectName, "-", "_")
	projectName = strings.TrimSpace(projectName)
	return strings.ToUpper(projectName)
}

// DetermineStorageRoot determines the root dir to store data.
func DetermineStorageRoot(projectID string) (string, error) {
	envVar := StorageRootEnvVar(projectID)
	envVal := os.Getenv(envVar)
	if envVal != "" {
		return envVal, nil
	}

	return DetermineConfigDir(projectID)
}
