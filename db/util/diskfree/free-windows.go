//go:build windows

package diskfree

import "golang.org/x/sys/windows"

// Bytes returns the bytes the caller can use on the volume holding dir.
func Bytes(dir string) (uint64, error) {
	// Ask the volume of dir for the bytes free to this caller.
	path, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var avail uint64
	if err := windows.GetDiskFreeSpaceEx(path, &avail, nil, nil); err != nil {
		return 0, err
	}
	return avail, nil
}
