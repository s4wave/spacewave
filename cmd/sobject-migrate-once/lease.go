package main

import (
	stderrors "errors"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/pkg/errors"
)

// requireOffline refuses any ready marker, including a stale copied marker.
func requireOffline(statePath string) error {
	// Inspect only the requested state's readiness marker.
	_, err := os.Lstat(filepath.Join(statePath, "spacewave.sock.ready"))
	if err == nil {
		return errors.New("spacewave.sock.ready exists; refuse to access this state")
	}
	if !os.IsNotExist(err) {
		return err
	}
	return nil
}

// acquireLease reserves the daemon's existing bbolt coordination file without writing.
// The bbolt dependency locks bytes [0,64) of runtime-lease.s4wave-lock-coord.
// Closing the returned descriptor relinquishes the process-scoped kernel lease.
func acquireLease(statePath string) (*os.File, error) {
	// Require the existing runtime lease rather than creating an uncoordinated root.
	file, err := os.OpenFile(filepath.Join(statePath, "runtime-lease.s4wave-lock-coord"), os.O_RDWR, 0)
	if err != nil {
		return nil, errors.Wrap(err, "open existing runtime lease")
	}

	// Attempt the same nonblocking kernel lock held by the serve runtime.
	lock := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: int16(io.SeekStart), Start: 0, Len: 64}
	if err := syscall.FcntlFlock(file.Fd(), syscall.F_SETLK, &lock); err != nil {
		return nil, stderrors.Join(errors.Wrap(err, "runtime lease is held or unavailable"), file.Close())
	}
	return file, nil
}
