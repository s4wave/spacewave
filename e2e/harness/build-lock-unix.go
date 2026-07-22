//go:build !js && unix

package harness

import (
	stderrors "errors"

	"golang.org/x/sys/unix"
)

func (l *BuildLock) tryLock() (bool, error) {
	err := unix.Flock(int(l.file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if stderrors.Is(err, unix.EWOULDBLOCK) || stderrors.Is(err, unix.EAGAIN) {
		return false, nil
	}
	return false, err
}

func (l *BuildLock) lock() error {
	for {
		err := unix.Flock(int(l.file.Fd()), unix.LOCK_EX)
		if stderrors.Is(err, unix.EINTR) {
			continue
		}
		return err
	}
}

func (l *BuildLock) unlock() error {
	return unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
}
