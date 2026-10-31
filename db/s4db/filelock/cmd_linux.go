package filelock

import "golang.org/x/sys/unix"

// Linux open file description locks belong to the descriptor, so a process
// sees its own other handles as conflicts and closing an unrelated
// descriptor keeps them.
const (
	// cmdSetLock takes a lock or fails.
	cmdSetLock = unix.F_OFD_SETLK
	// cmdSetLockWait takes a lock, waiting for conflicting holders.
	cmdSetLockWait = unix.F_OFD_SETLKW
	// cmdGetLock reports a conflicting lock.
	cmdGetLock = unix.F_OFD_GETLK
)
