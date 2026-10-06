//go:build linux

package s4db

import (
	"os"

	"golang.org/x/sys/unix"
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

// punch deallocates n bytes at off, keeping the file length.
func punch(f *os.File, off, n int64) error {
	return unix.Fallocate(int(f.Fd()), unix.FALLOC_FL_PUNCH_HOLE|unix.FALLOC_FL_KEEP_SIZE, off, n)
}

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

// watcher waits for writes to the database file with inotify.
type watcher struct {
	// fd is the inotify descriptor.
	fd int
	// wake is the pipe that stops wait.
	wake [2]int
	// buf receives events.
	buf []byte
}

// newWatcher watches the file at path.
func newWatcher(path string) (*watcher, error) {
	// Watch the file for writes.
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return nil, err
	}
	w := &watcher{fd: fd, buf: make([]byte, 64<<10)}
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
func (w *watcher) wait() bool {
	fds := []unix.PollFd{{Fd: int32(w.fd), Events: unix.POLLIN}, {Fd: int32(w.wake[0]), Events: unix.POLLIN}}
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

// stop wakes wait for the last time.
func (w *watcher) stop() {
	_, _ = unix.Write(w.wake[1], []byte{0})
}

// close releases the watcher.
func (w *watcher) close() {
	for _, fd := range []int{w.fd, w.wake[0], w.wake[1]} {
		if fd > 0 {
			_ = unix.Close(fd)
		}
	}
}
