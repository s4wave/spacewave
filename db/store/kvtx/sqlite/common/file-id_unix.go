//go:build unix

package common

import (
	"errors"
	"os"
	"syscall"
)

// fileID identifies a database file by its device and inode, which a hard
// link shares and a new file at the same path does not.
type fileID struct {
	dev, ino uint64
}

// statFileID returns the identity of the file at path.
func statFileID(path string) (fileID, error) {
	// Read the device and inode the path names now.
	info, err := os.Stat(path)
	if err != nil {
		return fileID{}, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileID{}, errors.New("no device and inode for " + path)
	}
	return fileID{dev: uint64(st.Dev), ino: st.Ino}, nil //nolint:gosec,unconvert // Dev is signed on some platforms and is an identifier, not a count.
}
