//go:build linux

package common

import (
	"os"

	"golang.org/x/sys/unix"
)

// syncFileData flushes a file's data and the metadata needed to read it,
// without its timestamps.
func syncFileData(f *os.File) error {
	return unix.Fdatasync(int(f.Fd()))
}
