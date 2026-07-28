//go:build !js

package cli_entrypoint

import (
	entrypoint_storagepath "github.com/s4wave/spacewave/bldr/entrypoint/storagepath"
)

// StatePathEnvVar returns the project-specific state path environment variable.
func StatePathEnvVar(projectID string) string {
	return entrypoint_storagepath.StatePathEnvVar(projectID)
}

// StatePathEnvVars returns the environment variables that override the state path.
func StatePathEnvVars(projectID string) []string {
	if projectID == "" {
		return []string{"BLDR_STATE_PATH"}
	}
	return []string{
		StatePathEnvVar(projectID),
		entrypoint_storagepath.StorageRootEnvVar(projectID),
		"BLDR_STATE_PATH",
	}
}

// DefaultStatePath returns the default state path for the CLI.
func DefaultStatePath(projectID string) string {
	if projectID == "" {
		return ".bldr"
	}
	statePath, err := entrypoint_storagepath.DetermineStorageRoot(projectID)
	if err != nil {
		return "." + projectID
	}
	return statePath
}
