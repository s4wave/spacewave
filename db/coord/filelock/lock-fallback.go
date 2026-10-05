//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly && !solaris && !windows

package filelock

import "os"

// lockFilesSupported reports that this platform has no advisory file locks;
// keyed scopes fall back to in-memory exclusion.
const lockFilesSupported = false

// tryLockFile takes the exclusive advisory lock on file without waiting.
func tryLockFile(*os.File) (bool, error) {
	return true, nil
}

// lockFile waits for the exclusive advisory lock on file.
func lockFile(*os.File) error {
	return nil
}

// unlockFile releases the advisory lock on file.
func unlockFile(*os.File) error {
	return nil
}
