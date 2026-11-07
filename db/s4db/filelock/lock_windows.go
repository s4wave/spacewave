package filelock

import (
	"os"

	"golang.org/x/sys/windows"
)

// Lock takes a write lock on the byte at off, waiting for other holders when
// wait is set. Without wait it reports false when another holder has it.
// Windows locks belong to the handle, and the system drops them when the
// process exits.
func Lock(f *os.File, off int64, wait bool) (bool, error) {
	// Fail at once on a held lock unless waiting.
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK)
	if !wait {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}

	// Take the lock; a violation means another holder has it.
	ov := overlapped(off)
	err := windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, &ov)
	switch err {
	case nil:
		return true, nil
	case windows.ERROR_LOCK_VIOLATION, windows.ERROR_IO_PENDING:
		return false, nil
	}
	return false, err
}

// Unlock releases the lock on the byte at off.
func Unlock(f *os.File, off int64) error {
	ov := overlapped(off)
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &ov)
}

// Held reports whether another holder has the lock on the byte at off.
// Windows cannot query a lock, so Held takes and drops it; a process taking
// that byte meanwhile moves on to the next free one.
func Held(f *os.File, off int64) (bool, error) {
	ok, err := Lock(f, off, false)
	if err != nil || !ok {
		return !ok, err
	}
	return false, Unlock(f, off)
}

// overlapped returns the OVERLAPPED that addresses the byte at off.
func overlapped(off int64) windows.Overlapped {
	return windows.Overlapped{Offset: uint32(off), OffsetHigh: uint32(off >> 32)} // #nosec G115 -- splits off into its halves.
}
