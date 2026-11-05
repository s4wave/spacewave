package common

// syncDir does nothing: Windows cannot flush a directory handle, and NTFS
// journals its directory entries.
func syncDir(string) error {
	return nil
}
