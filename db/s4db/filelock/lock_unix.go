//go:build darwin || linux

package filelock

import (
	"os"

	"golang.org/x/sys/unix"
)

// Lock takes a write lock on the byte at off, waiting for other holders when
// wait is set. Without wait it reports false when another holder has it. The
// file must be open for writing.
func Lock(f *os.File, off int64, wait bool) (bool, error) {
	// Choose the command, then retry it past interrupts.
	cmd := cmdSetLock
	if wait {
		cmd = cmdSetLockWait
	}
	lk := unix.Flock_t{Type: unix.F_WRLCK, Start: off, Len: 1}
	for {
		err := unix.FcntlFlock(f.Fd(), cmd, &lk)
		switch err {
		case nil:
			return true, nil
		case unix.EINTR:
			continue
		case unix.EAGAIN, unix.EACCES:
			return false, nil
		}
		return false, err
	}
}

// Unlock releases the lock on the byte at off.
func Unlock(f *os.File, off int64) error {
	lk := unix.Flock_t{Type: unix.F_UNLCK, Start: off, Len: 1}
	return unix.FcntlFlock(f.Fd(), cmdSetLock, &lk)
}

// Held reports whether another holder has the lock on the byte at off.
func Held(f *os.File, off int64) (bool, error) {
	lk := unix.Flock_t{Type: unix.F_WRLCK, Start: off, Len: 1}
	if err := unix.FcntlFlock(f.Fd(), cmdGetLock, &lk); err != nil {
		return false, err
	}
	return lk.Type != unix.F_UNLCK, nil
}
