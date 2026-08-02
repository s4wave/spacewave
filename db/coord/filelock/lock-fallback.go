//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly && !solaris && !windows

package filelock

import "os"

// lockFilesSupported reports that this platform has no advisory file locks;
// keyed scopes fall back to in-memory exclusion.
const lockFilesSupported = false

func tryLockFile(*os.File) (bool, error) {
	return true, nil
}

func unlockFile(*os.File) error {
	return nil
}
