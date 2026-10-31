//go:build darwin || linux

package s4db

import (
	"os"
	"syscall"
)

// identify returns the device and inode of f.
func identify(f *os.File) (fileID, error) {
	info, err := f.Stat()
	if err != nil {
		return fileID{}, err
	}
	st := info.Sys().(*syscall.Stat_t)
	return fileID{volume: uint64(st.Dev), index: st.Ino}, nil //nolint:unconvert,gosec // Dev is int32 on darwin and uint64 on linux.
}
