//go:build darwin || linux

package s4db

import (
	"os"

	"golang.org/x/sys/unix"
)

// lock takes a write lock on the byte at off, waiting for other holders when
// wait is set. Without wait it reports false when another holder has it.
func lock(f *os.File, off int64, wait bool) (bool, error) {
	cmd := cmdSetLock
	if wait {
		cmd = cmdSetLockWait
	}
	lk := unix.Flock_t{Type: unix.F_WRLCK, Whence: 0, Start: off, Len: 1}
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

// unlock releases the lock on the byte at off.
func unlock(f *os.File, off int64) error {
	lk := unix.Flock_t{Type: unix.F_UNLCK, Whence: 0, Start: off, Len: 1}
	return unix.FcntlFlock(f.Fd(), cmdSetLock, &lk)
}

// held reports whether another holder has the lock on the byte at off.
func held(f *os.File, off int64) (bool, error) {
	lk := unix.Flock_t{Type: unix.F_WRLCK, Whence: 0, Start: off, Len: 1}
	if err := unix.FcntlFlock(f.Fd(), cmdGetLock, &lk); err != nil {
		return false, err
	}
	return lk.Type != unix.F_UNLCK, nil
}
