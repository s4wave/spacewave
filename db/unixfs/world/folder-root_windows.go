//go:build windows

package unixfs_world

import (
	"os"

	"golang.org/x/sys/windows"
)

// folderRootID returns the identity of the file system holding the directory
// at path: its volume serial number.
func folderRootID(path string) (uint64, error) {
	// Open the directory itself.
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
	h, err := windows.CreateFile(p, 0, share, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return 0, &os.PathError{Op: "open", Path: path, Err: err}
	}
	defer windows.CloseHandle(h) //nolint:errcheck // a read-only handle.

	// Read its volume.
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return 0, &os.PathError{Op: "stat", Path: path, Err: err}
	}
	return uint64(info.VolumeSerialNumber), nil
}
