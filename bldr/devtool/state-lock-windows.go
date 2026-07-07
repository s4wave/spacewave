//go:build !js && windows

package devtool

import (
	stderrors "errors"

	"golang.org/x/sys/windows"
)

func (l *stateLock) tryLock() (bool, error) {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(
		windows.Handle(l.file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&overlapped,
	)
	if err == nil {
		return true, nil
	}
	if stderrors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return false, err
}

func (l *stateLock) lock() error {
	var overlapped windows.Overlapped
	return windows.LockFileEx(
		windows.Handle(l.file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0,
		1,
		0,
		&overlapped,
	)
}

func (l *stateLock) unlock() error {
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(
		windows.Handle(l.file.Fd()),
		0,
		1,
		0,
		&overlapped,
	)
}
