//go:build darwin || linux

package s4db

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// lock takes a write lock on the byte at off, waiting for other holders when
// wait is set. Without wait it reports false when another holder has it.
func (f *osFile) lock(off int64, wait bool) (bool, error) {
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

// unlock releases the lock on the byte at off.
func (f *osFile) unlock(off int64) error {
	lk := unix.Flock_t{Type: unix.F_UNLCK, Start: off, Len: 1}
	return unix.FcntlFlock(f.Fd(), cmdSetLock, &lk)
}

// held reports whether another holder has the lock on the byte at off.
func (f *osFile) held(off int64) (bool, error) {
	lk := unix.Flock_t{Type: unix.F_WRLCK, Start: off, Len: 1}
	if err := unix.FcntlFlock(f.Fd(), cmdGetLock, &lk); err != nil {
		return false, err
	}
	return lk.Type != unix.F_UNLCK, nil
}

// identify returns the device and inode of f.
func identify(f *os.File) (fileID, error) {
	info, err := f.Stat()
	if err != nil {
		return fileID{}, err
	}
	st := info.Sys().(*syscall.Stat_t)
	return fileID{volume: uint64(st.Dev), index: st.Ino}, nil //nolint:unconvert,gosec // Dev is int32 on darwin and uint64 on linux.
}
