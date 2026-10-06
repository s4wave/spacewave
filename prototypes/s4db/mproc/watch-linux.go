//go:build linux

package main

import (
	"golang.org/x/sys/unix"
)

// watch waits for writes to a file with inotify.
type watch struct {
	// fd is the inotify descriptor.
	fd int
	// buf receives events.
	buf []byte
}

// newWatch adds a modify watch on path.
func newWatch(path string) (*watch, error) {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC)
	if err != nil {
		return nil, err
	}
	if _, err := unix.InotifyAddWatch(fd, path, unix.IN_MODIFY); err != nil {
		return nil, err
	}
	return &watch{fd: fd, buf: make([]byte, 64<<10)}, nil
}

// wait blocks until the file changes, draining queued events.
func (w *watch) wait() error {
	for {
		_, err := unix.Read(w.fd, w.buf)
		if err == unix.EINTR {
			continue
		}
		return err
	}
}
