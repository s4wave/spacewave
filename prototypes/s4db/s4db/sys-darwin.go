//go:build darwin

package s4db

import (
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// flushDurable makes earlier writes durable on the drive.
func flushDurable(f *os.File) error {
	_, err := unix.FcntlInt(f.Fd(), unix.F_FULLFSYNC, 0)
	return err
}

// flushOrdered orders earlier writes before later ones without waiting for
// the drive to make them durable.
func flushOrdered(f *os.File) error {
	_, err := unix.FcntlInt(f.Fd(), unix.F_BARRIERFSYNC, 0)
	return err
}

// fpunchhole is struct fpunchhole from sys/fcntl.h.
type fpunchhole struct {
	flags    uint32
	reserved uint32
	offset   int64
	length   int64
}

// punch deallocates n bytes at off, keeping the file length.
func punch(f *os.File, off, n int64) error {
	arg := fpunchhole{offset: off, length: n}
	_, _, errno := unix.Syscall(unix.SYS_FCNTL, f.Fd(), unix.F_PUNCHHOLE, uintptr(unsafe.Pointer(&arg)))
	if errno != 0 {
		return errno
	}
	return nil
}

// Darwin has only process-scoped record locks: closing any descriptor of the
// file drops every lock the process holds on it, and a process does not see
// its own locks as conflicts. The database keeps one descriptor per file and
// one handle per process.
const (
	// cmdSetLock takes a lock or fails.
	cmdSetLock = unix.F_SETLK
	// cmdSetLockWait takes a lock, waiting for conflicting holders.
	cmdSetLockWait = unix.F_SETLKW
	// cmdGetLock reports a conflicting lock.
	cmdGetLock = unix.F_GETLK
)

// watcher waits for writes to the database file with kqueue.
type watcher struct {
	// kq is the kqueue.
	kq int
	// fd is the event-only descriptor of the file.
	fd int
	// wake is the pipe that stops wait.
	wake [2]int
}

// newWatcher watches the file at path.
func newWatcher(path string) (*watcher, error) {
	// Open the file for events only, so the descriptor holds no locks.
	w := &watcher{kq: -1, fd: -1}
	var err error
	if w.fd, err = unix.Open(path, unix.O_EVTONLY, 0); err != nil {
		return nil, err
	}

	// Register writes to the file and the stop pipe with a kqueue.
	if err = unix.Pipe(w.wake[:]); err != nil {
		w.close()
		return nil, err
	}
	if w.kq, err = unix.Kqueue(); err != nil {
		w.close()
		return nil, err
	}
	evs := []unix.Kevent_t{
		{Ident: uint64(w.fd), Filter: unix.EVFILT_VNODE, Flags: unix.EV_ADD | unix.EV_CLEAR, Fflags: unix.NOTE_WRITE | unix.NOTE_EXTEND},
		{Ident: uint64(w.wake[0]), Filter: unix.EVFILT_READ, Flags: unix.EV_ADD},
	}
	if _, err := unix.Kevent(w.kq, evs, nil, nil); err != nil {
		w.close()
		return nil, err
	}
	return w, nil
}

// wait blocks until the file changes, reporting false once stopped.
func (w *watcher) wait() bool {
	out := make([]unix.Kevent_t, 2)
	for {
		n, err := unix.Kevent(w.kq, nil, out, nil)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return false
		}
		for _, ev := range out[:n] {
			if ev.Ident == uint64(w.wake[0]) {
				return false
			}
		}
		return true
	}
}

// stop wakes wait for the last time.
func (w *watcher) stop() {
	_, _ = unix.Write(w.wake[1], []byte{0})
}

// close releases the watcher. It drops the process's record locks on the
// file, so the database closes it last.
func (w *watcher) close() {
	for _, fd := range []int{w.kq, w.fd, w.wake[0], w.wake[1]} {
		if fd > 0 {
			_ = unix.Close(fd)
		}
	}
}
