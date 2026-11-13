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

// writeBackFile writes a file's dirty pages to the device and waits for them,
// without the journal commit a flush forces: a later flush of the file finds
// little left to write, and writers appending to other files on the same
// filesystem never wait behind it. It makes nothing durable by itself.
func writeBackFile(f *os.File) error {
	return unix.SyncFileRange(int(f.Fd()), 0, 0, unix.SYNC_FILE_RANGE_WAIT_BEFORE|unix.SYNC_FILE_RANGE_WRITE|unix.SYNC_FILE_RANGE_WAIT_AFTER)
}
