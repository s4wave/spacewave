//go:build !windows && !js && !wasip1

package volume_s4db

import (
	"errors"
	"os"
	"strconv"
	"syscall"
)

// fileIdentity names the file fi describes by its device and inode, which a
// conversion's claim records.
func fileIdentity(fi os.FileInfo) (string, error) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return "", errors.New("file identity is not available")
	}
	return strconv.FormatUint(uint64(st.Dev), 10) + ":" + strconv.FormatUint(st.Ino, 10), nil //nolint:unconvert,gosec // Dev is int32 on darwin and uint64 on linux.
}

// linkCount returns the number of names the file fi describes has.
func linkCount(fi os.FileInfo) (uint64, error) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("file link count is not available")
	}
	return uint64(st.Nlink), nil //nolint:unconvert // Nlink is uint16 on darwin and uint64 on linux.
}
