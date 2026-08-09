package dist_entrypoint

import entrypoint_storagepath "github.com/s4wave/spacewave/bldr/entrypoint/storagepath"

// DetermineStorageRoot determines the root dir to store data.
func DetermineStorageRoot(projectID string) (string, error) {
	return entrypoint_storagepath.DetermineStorageRoot(projectID)
}
