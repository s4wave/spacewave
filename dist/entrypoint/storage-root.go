package dist_entrypoint

import entrypoint_storagepath "github.com/aperturerobotics/bldr/entrypoint/storagepath"

// StorageRootEnvVar returns the environment variable for the storage root.
// PROJECT_NAME_DATA_DIR
func StorageRootEnvVar(projectID string) string {
	return entrypoint_storagepath.StorageRootEnvVar(projectID)
}

// DetermineStorageRoot determines the root dir to store data.
func DetermineStorageRoot(projectID string) (string, error) {
	return entrypoint_storagepath.DetermineStorageRoot(projectID)
}
