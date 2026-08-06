//go:build windows

package npm

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

const errorLockViolation windows.Errno = 0x21

func (l *installLock) tryLock() (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held {
		return true, nil
	}
	if l.file == nil {
		file, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return false, err
		}
		l.file = file
	}
	err := windows.LockFileEx(
		windows.Handle(l.file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&windows.Overlapped{},
	)
	if err != nil {
		if errors.Is(err, errorLockViolation) || errors.Is(err, windows.ERROR_IO_PENDING) {
			return false, nil
		}
		if !errors.Is(err, windows.Errno(0)) {
			return false, err
		}
	}
	l.held = true
	return true, nil
}

func (l *installLock) Unlock() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.held || l.file == nil {
		return nil
	}
	err := windows.UnlockFileEx(
		windows.Handle(l.file.Fd()),
		0,
		1,
		0,
		&windows.Overlapped{},
	)
	if err != nil && !errors.Is(err, windows.Errno(0)) {
		return err
	}
	l.held = false
	closeErr := l.file.Close()
	l.file = nil
	return closeErr
}
