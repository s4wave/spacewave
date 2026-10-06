package s4db

import (
	"os"

	"golang.org/x/sys/unix"
)

// fileWatcher waits for writes to the database file with inotify.
type fileWatcher struct {
	// fd is the inotify descriptor.
	fd int
	// wake is the pipe that stops wait.
	wake [2]int
	// buf receives events.
	buf []byte
}

// newFileWatcher watches the file at path. Every write raises an inotify event,
// so the slot is unused.
func newFileWatcher(_ *os.File, path string, _ int) (*fileWatcher, error) {
	// Watch the file for writes.
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return nil, err
	}
	w := &fileWatcher{fd: fd, wake: [2]int{-1, -1}, buf: make([]byte, 64<<10)}
	if _, err := unix.InotifyAddWatch(fd, path, unix.IN_MODIFY); err != nil {
		w.close()
		return nil, err
	}

	// Open the pipe that stops wait.
	if err := unix.Pipe2(w.wake[:], unix.O_CLOEXEC); err != nil {
		w.close()
		return nil, err
	}
	return w, nil
}

// wait blocks until the file changes, reporting false once stopped.
func (w *fileWatcher) wait() bool {
	fds := []unix.PollFd{{Fd: int32(w.fd), Events: unix.POLLIN}, {Fd: int32(w.wake[0]), Events: unix.POLLIN}} // #nosec G115 -- descriptors fit in an int.
	for {
		_, err := unix.Poll(fds, -1)
		if err == unix.EINTR {
			continue
		}
		if err != nil || fds[1].Revents != 0 {
			return false
		}

		// Drain queued events; one tail serves them all.
		for {
			if _, err := unix.Read(w.fd, w.buf); err != nil {
				break
			}
		}
		return true
	}
}

// notify tells other processes the file changed. The kernel already did.
func (w *fileWatcher) notify() {}

// stop wakes wait for the last time.
func (w *fileWatcher) stop() {
	_, _ = unix.Write(w.wake[1], []byte{0})
}

// close releases the watcher.
func (w *fileWatcher) close() {
	for _, fd := range []int{w.fd, w.wake[0], w.wake[1]} {
		if fd >= 0 {
			_ = unix.Close(fd)
		}
	}
}
