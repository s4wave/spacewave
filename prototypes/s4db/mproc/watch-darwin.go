//go:build darwin

package main

import (
	"golang.org/x/sys/unix"
)

// watch waits for writes to a file with kqueue.
type watch struct {
	// kq is the kqueue descriptor.
	kq int
}

// newWatch registers a vnode write filter on path.
func newWatch(path string) (*watch, error) {
	// Open the file for events only and register it with a new kqueue.
	fd, err := unix.Open(path, unix.O_EVTONLY, 0)
	if err != nil {
		return nil, err
	}
	kq, err := unix.Kqueue()
	if err != nil {
		return nil, err
	}
	ev := unix.Kevent_t{Ident: uint64(fd), Filter: unix.EVFILT_VNODE, Flags: unix.EV_ADD | unix.EV_CLEAR, Fflags: unix.NOTE_WRITE | unix.NOTE_EXTEND}
	if _, err := unix.Kevent(kq, []unix.Kevent_t{ev}, nil, nil); err != nil {
		return nil, err
	}
	return &watch{kq: kq}, nil
}

// wait blocks until the file changes.
func (w *watch) wait() error {
	out := make([]unix.Kevent_t, 1)
	for {
		n, err := unix.Kevent(w.kq, nil, out, nil)
		if err == unix.EINTR {
			continue
		}
		if err != nil || n > 0 {
			return err
		}
	}
}
