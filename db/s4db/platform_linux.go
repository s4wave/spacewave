package s4db

import (
	"os"

	"golang.org/x/sys/unix"
)

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

// flushDurable makes earlier writes durable on the drive.
func flushDurable(f *os.File) error {
	return unix.Fdatasync(int(f.Fd()))
}

// flushOrdered orders earlier writes before later ones. Linux has no write
// barrier short of a full flush, so ordered commits rely on record
// checksums: recovery keeps the longest valid prefix of the log.
func flushOrdered(*os.File) error {
	return nil
}

// flushBarrier orders earlier writes before later ones. Linux has no
// cheaper ordering than a full flush, so the barrier is durable.
func flushBarrier(f *os.File) (bool, error) {
	return true, flushDurable(f)
}

// punch deallocates n bytes at off, keeping the file length.
func punch(f *os.File, off, n int64) error {
	return unix.Fallocate(int(f.Fd()), unix.FALLOC_FL_PUNCH_HOLE|unix.FALLOC_FL_KEEP_SIZE, off, n)
}
