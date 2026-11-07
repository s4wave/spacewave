package filelock

import "golang.org/x/sys/unix"

// Darwin has only process-scoped record locks: closing any descriptor of the
// file drops every lock the process holds on it, and a process does not see
// its own locks as conflicts. A caller keeps one descriptor per file and one
// handle per process.
const (
	// cmdSetLock takes a lock or fails.
	cmdSetLock = unix.F_SETLK
	// cmdSetLockWait takes a lock, waiting for conflicting holders.
	cmdSetLockWait = unix.F_SETLKW
	// cmdGetLock reports a conflicting lock.
	cmdGetLock = unix.F_GETLK
)
