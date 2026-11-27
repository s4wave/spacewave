//go:build !unix

package common

import "os"

// fileID identifies a database file by its path, where the platform reports
// no device and inode, so a hard link has its own identity.
type fileID struct {
	path string
}

// statFileID returns the identity of the file at path, which must exist.
func statFileID(path string) (fileID, error) {
	if _, err := os.Stat(path); err != nil {
		return fileID{}, err
	}
	return fileID{path: path}, nil
}
