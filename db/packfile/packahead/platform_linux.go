package packahead

import (
	"os"

	"golang.org/x/sys/unix"
)

// syncData makes the file's written data durable on the drive, with its size
// but not its other metadata.
func syncData(f *os.File) error {
	return unix.Fdatasync(int(f.Fd()))
}
