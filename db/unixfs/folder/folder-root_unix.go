//go:build unix

package folder

import (
	"os"

	"golang.org/x/sys/unix"
)

// folderRootID returns the identity of the file system holding the directory
// at path: its device number.
func folderRootID(path string) (uint64, error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return 0, &os.PathError{Op: "stat", Path: path, Err: err}
	}
	return devID(st.Dev), nil
}

// devID widens a device number, whose type differs between platforms.
func devID[T int32 | uint32 | uint64](dev T) uint64 {
	return uint64(dev) //nolint:gosec // a device number is not negative.
}
