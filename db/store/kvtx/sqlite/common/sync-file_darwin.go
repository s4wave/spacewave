//go:build darwin

package common

import (
	"os"

	"golang.org/x/sys/unix"
)

// syncFileData flushes a file's data and metadata to stable storage.
func syncFileData(f *os.File) error {
	return f.Sync()
}

// writeBackFile writes a file's dirty pages to the device with a plain
// fsync, which waits for them but does not flush the device's cache as
// os.File.Sync does with F_FULLFSYNC: a later flush of the file finds little
// left to write. It makes nothing durable by itself.
func writeBackFile(f *os.File) error {
	return unix.Fsync(int(f.Fd()))
}
