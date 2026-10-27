//go:build windows

package volume_s4db

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

// freeDiskBytes returns the space free to this process on the volume holding
// the file at path.
func freeDiskBytes(path string) (uint64, error) {
	// Name the directory holding the file.
	dir, err := windows.UTF16PtrFromString(filepath.Dir(path))
	if err != nil {
		return 0, err
	}

	// Read the space free to the caller.
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(dir, &free, &total, &totalFree); err != nil {
		return 0, err
	}
	return free, nil
}
