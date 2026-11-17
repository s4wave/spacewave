//go:build darwin || linux

package main

import (
	"os"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// lockScratch takes the lock beside the scratch file at path, so one step of
// the cycle runs at a time: a restore and a purge, or a removal and a
// restore, act on the same records and objects and must not interleave. It
// fails at once when another step holds the lock. Closing the file releases
// it, as does the process's exit.
func lockScratch(path string) (*os.File, error) {
	// Open the lock file beside the scratch file.
	f, err := os.OpenFile(path+".lock", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}

	// Take it, failing when another step has it.
	for {
		err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) //nolint:gosec // A file descriptor fits an int.
		if err != unix.EINTR {
			break
		}
	}
	if err != nil {
		_ = f.Close()
		if err == unix.EWOULDBLOCK {
			return nil, errors.Errorf("another step of the cycle holds %s", f.Name())
		}
		return nil, errors.Wrap(err, "lock the scratch file")
	}
	return f, nil
}
