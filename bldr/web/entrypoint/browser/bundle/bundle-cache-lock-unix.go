//go:build !js && unix

package entrypoint_browser_bundle

import (
	stderrors "errors"

	"golang.org/x/sys/unix"
)

func (lock *bundleCacheLock) lock() error {
	for {
		err := unix.Flock(int(lock.file.Fd()), unix.LOCK_EX)
		if stderrors.Is(err, unix.EINTR) {
			continue
		}
		return err
	}
}

func (lock *bundleCacheLock) unlock() error {
	return unix.Flock(int(lock.file.Fd()), unix.LOCK_UN)
}
