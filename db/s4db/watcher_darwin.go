package s4db

import (
	"os"

	"golang.org/x/sys/unix"
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

// newWatcher watches the file at path. Every write raises a kqueue event,
// so the slot is unused.
func newWatcher(_ *os.File, path string, _ int) (*watcher, error) {
	// Open the file for events only.
	w := &watcher{kq: -1, fd: -1, wake: [2]int{-1, -1}}
	var err error
	if w.fd, err = unix.Open(path, unix.O_EVTONLY|unix.O_CLOEXEC, 0); err != nil {
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
		{Ident: ident(w.fd), Filter: unix.EVFILT_VNODE, Flags: unix.EV_ADD | unix.EV_CLEAR, Fflags: unix.NOTE_WRITE | unix.NOTE_EXTEND},
		{Ident: ident(w.wake[0]), Filter: unix.EVFILT_READ, Flags: unix.EV_ADD},
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
			if ev.Ident == ident(w.wake[0]) {
				return false
			}
		}
		return true
	}
}

// notify tells other processes the file changed. The kernel already did.
func (w *watcher) notify() {}

// stop wakes wait for the last time.
func (w *watcher) stop() {
	_, _ = unix.Write(w.wake[1], []byte{0})
}

// close releases the watcher. It drops the process's record locks on the
// file, so the database closes it last.
func (w *watcher) close() {
	for _, fd := range []int{w.kq, w.fd, w.wake[0], w.wake[1]} {
		if fd >= 0 {
			_ = unix.Close(fd)
		}
	}
}

// ident returns the kevent identifier of descriptor fd.
func ident(fd int) uint64 {
	return uint64(fd) // #nosec G115 -- open descriptors are not negative.
}
