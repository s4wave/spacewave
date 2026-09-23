package storagepath

import (
	"os"
	"path/filepath"
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

// ResolveStatePath makes statePath absolute, creates it, and publishes it
// with the optional explicit socket path to the project's environment
// variables. Logs, bus-hosted components, and child processes then scope
// their default paths to the invocation's state root instead of the shared
// process default. socketPath may be empty when no explicit socket path was
// requested. Returns the absolute state root.
func ResolveStatePath(projectID, statePath, socketPath string) (string, error) {
	root, err := filepath.Abs(statePath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	if err := os.Setenv(StatePathEnvVar(projectID), root); err != nil {
		return "", err
	}
	if socketPath != "" {
		if err := os.Setenv(SocketPathEnvVar(projectID), socketPath); err != nil {
			return "", err
		}
	}
	return root, nil
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
//
// The resolved state path takes precedence over the data directory so every
// component of one invocation (stores, logs, and helpers) shares the root the
// CLI selected. Without either override the platform config dir is used.
func DetermineStorageRoot(projectID string) (string, error) {
	for _, envVar := range []string{StatePathEnvVar(projectID), StorageRootEnvVar(projectID)} {
		if envVal := os.Getenv(envVar); envVal != "" {
			return envVal, nil
		}
	}
	return DetermineConfigDir(projectID)
}
