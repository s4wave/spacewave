package common

// keepSyncedWAL is false: Windows cannot delete a file a handle without
// FILE_SHARE_DELETE holds open, so SQLite could not delete its WAL, and with
// no directory flush there is no entry flush to skip.
const keepSyncedWAL = false

// syncDir does nothing: Windows cannot flush a directory handle, and NTFS
// journals its directory entries.
func syncDir(string) error {
	return nil
}
