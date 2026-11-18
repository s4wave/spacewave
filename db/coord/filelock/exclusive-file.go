package filelock

import (
	"os"

	"github.com/pkg/errors"
)

// TryLockExclusive takes an advisory, non-blocking exclusive lock on an open
// file. Closing the file releases the lock. It creates no separate lock file.
// On platforms without advisory locks it returns an error.
func TryLockExclusive(file *os.File) (bool, error) {
	// A caller using this lock requires cross-process exclusion.
	if !lockFilesSupported {
		return false, errors.New("exclusive file locking is unavailable")
	}
	return tryLockFile(file)
}
