package s4db

import (
	"golang.org/x/sys/unix"
)

// flushDurable makes earlier writes durable on the drive.
func (f *osFile) flushDurable() error {
	_, err := unix.FcntlInt(f.Fd(), unix.F_FULLFSYNC, 0)
	return err
}

// writeback flushes durably: fsync would leave the pages in the drive's
// cache, where a commit's F_FULLFSYNC would still find them.
func (f *osFile) writeback() error {
	return f.flushDurable()
}

// lowerThreadPriority puts the calling thread in the background band, which
// throttles its CPU and disk use while other work runs.
func lowerThreadPriority() error {
	return unix.Setpriority(unix.PRIO_DARWIN_THREAD, 0, unix.PRIO_DARWIN_BG)
}
