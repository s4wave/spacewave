//go:build !js && unix

package devtool

import (
	stderrors "errors"

	"golang.org/x/sys/unix"
)

func (l *stateLock) tryLock() (bool, error) {
	err := unix.Flock(int(l.file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if stderrors.Is(err, unix.EWOULDBLOCK) || stderrors.Is(err, unix.EAGAIN) {
		return false, nil
	}
	return false, err
}

func (l *stateLock) lock() error {
	for {
		err := unix.Flock(int(l.file.Fd()), unix.LOCK_EX)
		if stderrors.Is(err, unix.EINTR) {
			continue
		}
		return err
	}
}

func (l *stateLock) unlock() error {
	return unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
}
