package packahead

import (
	"os"

	"golang.org/x/sys/unix"
)

// syncData makes the file's written data durable on the drive. A plain fsync
// on darwin leaves data in the drive's cache, so this asks for F_FULLFSYNC.
func syncData(f *os.File) error {
	_, err := unix.FcntlInt(f.Fd(), unix.F_FULLFSYNC, 0)
	return err
}
