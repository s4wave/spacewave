//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly || solaris

package filelock

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// lockFilesSupported reports that advisory file locks exist on this platform.
const lockFilesSupported = true

// tryLockFile takes the exclusive advisory lock on file without waiting.
func tryLockFile(file *os.File) (bool, error) {
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// lockFile waits for the exclusive advisory lock on file.
func lockFile(file *os.File) error {
	for {
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX)
		if !errors.Is(err, unix.EINTR) {
			return err
		}
	}
}

// unlockFile releases the advisory lock on file.
func unlockFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
