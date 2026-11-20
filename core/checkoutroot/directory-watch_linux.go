package checkoutroot

import (
	"context"
	"encoding/binary"
	"os"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

// directoryWatchKey identifies the inode that inotify shares among path aliases.
type directoryWatchKey struct {
	// device separates inodes belonging to different filesystems.
	device uint64
	// inode identifies a directory on that device.
	inode uint64
}

// directoryWatchName is the watch descriptor in each inotify event.
type directoryWatchName int32

// directoryWatchSource reads one inotify queue for a checkout root.
type directoryWatchSource struct {
	// fd is the descriptor used by inotify registration syscalls.
	fd int
	// file owns fd and reads through the runtime poller.
	file *os.File
}

// directoryKey identifies the open directory independently of its current name.
func directoryKey(_ *os.Root, _ string, info os.FileInfo) (directoryWatchKey, error) {
	stat := info.Sys().(*syscall.Stat_t)
	return directoryWatchKey{device: stat.Dev, inode: stat.Ino}, nil
}

// openDirectoryWatchSource opens an inotify queue with close-interruptible reads.
func openDirectoryWatchSource() (*directoryWatchSource, error) {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return nil, err
	}
	return &directoryWatchSource{fd: fd, file: os.NewFile(uintptr(fd), "inotify")}, nil //nolint:gosec // inotify returns a nonnegative fd on success.
}

// add watches the pinned directory for entry changes and settled file writes.
func (s *directoryWatchSource) add(_ directoryWatchKey, dir *os.File, _ os.FileInfo) (directoryWatchName, error) {
	// The descriptor path resolves the same inode even while the host renames it.
	name := "/proc/self/fd/" + strconv.FormatUint(uint64(dir.Fd()), 10)
	wd, err := unix.InotifyAddWatch(s.fd, name,
		unix.IN_ONLYDIR|unix.IN_CREATE|unix.IN_DELETE|unix.IN_MOVED_FROM|unix.IN_MOVED_TO|
			unix.IN_DELETE_SELF|unix.IN_MOVE_SELF|unix.IN_CLOSE_WRITE)
	return directoryWatchName(wd), err //nolint:gosec // inotify returns a signed 32-bit watch descriptor.
}

// remove releases a descriptor that may already have been retired by inotify.
func (s *directoryWatchSource) remove(wd directoryWatchName) {
	_, _ = unix.InotifyRmWatch(s.fd, uint32(wd)) //nolint:gosec // registered watch descriptors are nonnegative.
}

// close interrupts reads and releases all remaining watch descriptors.
func (s *directoryWatchSource) close() error {
	return s.file.Close()
}

// run dispatches inotify records until the source closes or a read fails.
func (s *directoryWatchSource) run(ctx context.Context, w *DirectoryWatcher) error {
	// A read blocks in the runtime poller until an event or source close.
	buf := make([]byte, 64<<10)
	for ctx.Err() == nil {
		n, err := s.file.Read(buf)
		if err != nil {
			return err
		}

		// Each kernel record is complete and routes by its watch descriptor.
		for off := 0; off < n; {
			event := buf[off:n]
			wd := directoryWatchName(binary.NativeEndian.Uint32(event)) //nolint:gosec // the kernel writes a signed 32-bit descriptor.
			mask := binary.NativeEndian.Uint32(event[4:])
			off += unix.SizeofInotifyEvent + int(binary.NativeEndian.Uint32(event[12:]))
			switch {
			case mask&unix.IN_Q_OVERFLOW != 0:
				w.overflow()
			case mask&(unix.IN_DELETE_SELF|unix.IN_MOVE_SELF|unix.IN_UNMOUNT) != 0:
				w.dispatch(wd, true)
			case mask&unix.IN_IGNORED == 0:
				w.dispatch(wd, false)
			}
		}
	}
	return nil
}
