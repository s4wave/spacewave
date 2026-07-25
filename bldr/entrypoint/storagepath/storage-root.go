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

// StatePathEnvVar returns the environment variable that carries the resolved
// project state path (e.g. "spacewave" -> "SPACEWAVE_STATE_PATH").
func StatePathEnvVar(projectID string) string {
	return projectIDPrefix(projectID) + "_STATE_PATH"
}

// SocketPathEnvVar returns the environment variable that overrides the
// daemon socket path (e.g. "spacewave" -> "SPACEWAVE_SOCKET_PATH").
func SocketPathEnvVar(projectID string) string {
	return projectIDPrefix(projectID) + "_SOCKET_PATH"
}

// LogRetentionDaysEnvVar returns the environment variable that overrides
// the on-disk log retention duration (in days) for the given project.
func LogRetentionDaysEnvVar(projectID string) string {
	return projectIDPrefix(projectID) + "_LOG_RETENTION_DAYS"
}

// projectIDAllowedChars matches the run of characters retained when
// sanitizing a projectID into an environment-variable prefix.
var projectIDAllowedChars = regexp.MustCompile(`[a-zA-Z0-9_-]+`)

// projectIDPrefix sanitizes projectID into the upper-cased prefix used by
// project-scoped environment variables (e.g. "spacewave" -> "SPACEWAVE").
func projectIDPrefix(projectID string) string {
	matches := projectIDAllowedChars.FindAllString(projectID, -1)
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
