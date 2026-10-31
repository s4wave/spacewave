package s4db

import (
	"golang.org/x/sys/unix"
)

// Darwin has only process-scoped record locks: closing any descriptor of the
// file drops every lock the process holds on it, and a process does not see
// its own locks as conflicts. The database keeps one descriptor per file and
// one handle per process.
const (
	// cmdSetLock takes a lock or fails.
	cmdSetLock = unix.F_SETLK
	// cmdSetLockWait takes a lock, waiting for conflicting holders.
	cmdSetLockWait = unix.F_SETLKW
	// cmdGetLock reports a conflicting lock.
	cmdGetLock = unix.F_GETLK
)

// flushDurable makes earlier writes durable on the drive.
func (f *osFile) flushDurable() error {
	_, err := unix.FcntlInt(f.Fd(), unix.F_FULLFSYNC, 0)
	return err
}

// lowerThreadPriority puts the calling thread in the background band, which
// throttles its CPU and disk use while other work runs.
func lowerThreadPriority() error {
	return unix.Setpriority(unix.PRIO_DARWIN_THREAD, 0, unix.PRIO_DARWIN_BG)
}
