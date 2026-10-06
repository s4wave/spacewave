package s4db

import (
	"os"

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
func flushDurable(f *os.File) error {
	_, err := unix.FcntlInt(f.Fd(), unix.F_FULLFSYNC, 0)
	return err
}

// flushOrdered orders earlier writes before later ones without waiting for
// the drive to make them durable.
func flushOrdered(f *os.File) error {
	_, err := unix.FcntlInt(f.Fd(), unix.F_BARRIERFSYNC, 0)
	return err
}

// flushBarrier orders earlier writes before later ones, reporting that it
// did not make them durable.
func flushBarrier(f *os.File) (bool, error) {
	return false, flushOrdered(f)
}

// punch deallocates n bytes at off, keeping the file length. struct
// fpunchhole shares its layout with the leading fields of struct fstore,
// flags, a reserved word, offset and length, so the fstore wrapper passes it.
func punch(f *os.File, off, n int64) error {
	return unix.FcntlFstore(f.Fd(), unix.F_PUNCHHOLE, &unix.Fstore_t{Offset: off, Length: n})
}
