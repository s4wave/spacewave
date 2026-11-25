package filelock

import (
	"errors"
	"os"
)

// TryOpen creates or opens path and tries to lock it exclusively without
// waiting. A nil release with no error means another opener holds the lock.
// The caller must release a held lock once and leave the lock file in place.
func TryOpen(path string) (release func(), err error) {
	// Require kernel exclusion before opening a lock file.
	if !lockFilesSupported {
		return nil, errors.ErrUnsupported
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) // #nosec G703 -- the caller chooses the lock file.
	if err != nil {
		return nil, err
	}

	// Close a failed attempt immediately. Explicitly unlock before closing
	// a successful attempt so Windows can grant the next lock at once.
	locked, err := tryLockFile(file)
	if err != nil || !locked {
		_ = file.Close()
		return nil, err
	}
	return func() {
		_ = unlockFile(file)
		_ = file.Close()
	}, nil
}
