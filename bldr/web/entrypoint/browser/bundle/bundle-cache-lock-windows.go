//go:build !js && windows

package entrypoint_browser_bundle

import (
	"golang.org/x/sys/windows"
)

func (lock *bundleCacheLock) lock() error {
	var overlapped windows.Overlapped
	return windows.LockFileEx(
		windows.Handle(lock.file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0,
		1,
		0,
		&overlapped,
	)
}

func (lock *bundleCacheLock) unlock() error {
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(
		windows.Handle(lock.file.Fd()),
		0,
		1,
		0,
		&overlapped,
	)
}
