//go:build windows

package extsort

import "golang.org/x/sys/windows"

// freeBytes returns the bytes the caller can use on dir's volume.
func freeBytes(dir string) (int64, error) {
	// Ask the volume of dir for the bytes free to this caller.
	path, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var avail uint64
	if err := windows.GetDiskFreeSpaceEx(path, &avail, nil, nil); err != nil {
		return 0, err
	}
	return int64(avail), nil //nolint:gosec // a volume's free bytes fit int64.
}
