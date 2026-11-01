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
	// Use the platform's normal byte-lock ownership.
	return lockWithCommands(f, off, wait, cmdSetLock, cmdSetLockWait)
}

// lockWithCommands acquires a byte with the selected ownership semantics.
func lockWithCommands(f *os.File, off int64, wait bool, set, setWait int) (bool, error) {
	// Choose the command, then retry it past interrupts.
	cmd := set
	if wait {
		cmd = setWait
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
	return unlockWithCommand(f, off, cmdSetLock)
}

// unlockWithCommand releases a byte with the matching ownership semantics.
func unlockWithCommand(f *os.File, off int64, cmd int) error {
	lk := unix.Flock_t{Type: unix.F_UNLCK, Start: off, Len: 1}
	return unix.FcntlFlock(f.Fd(), cmd, &lk)
}

// Held reports whether another holder has the lock on the byte at off.
func Held(f *os.File, off int64) (bool, error) {
	return heldWithCommand(f, off, cmdGetLock)
}

// heldWithCommand queries conflicts with the selected ownership semantics.
func heldWithCommand(f *os.File, off int64, cmd int) (bool, error) {
	lk := unix.Flock_t{Type: unix.F_WRLCK, Start: off, Len: 1}
	if err := unix.FcntlFlock(f.Fd(), cmd, &lk); err != nil {
		return false, err
	}
	return lk.Type != unix.F_UNLCK, nil
}
