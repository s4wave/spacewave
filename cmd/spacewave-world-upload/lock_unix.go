//go:build unix

package main

import (
	"os"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// lockFile takes an exclusive non-blocking flock on f, held until f is
// closed. It fails at once when another open file holds it.
func lockFile(f *os.File) error {
	// Retry past interrupts; a held lock is an error at once.
	for {
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		switch err {
		case nil:
			return nil
		case unix.EINTR:
			continue
		case unix.EWOULDBLOCK:
			return errors.New("locked by another run; refusing to start")
		}
		return errors.Wrap(err, "flock")
	}
}
