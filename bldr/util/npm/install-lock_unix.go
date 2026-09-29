//go:build unix

package npm

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func (l *installLock) tryLock() (bool, error) {
	// Serialize lock attempts and short-circuit when already held.
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held {
		return true, nil
	}

	// Open the lock file if it does not exist yet.
	if l.file == nil {
		file, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDONLY, 0o600)
		if err != nil {
			return false, err
		}
		l.file = file
	}

	// Take an exclusive non-blocking flock on the lock file.
	if err := unix.Flock(int(l.file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) {
			return false, nil
		}
		return false, err
	}
	l.held = true
	return true, nil
}

func (l *installLock) Unlock() error {
	// Serialize unlock attempts and ignore a lock that is not held.
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.held || l.file == nil {
		return nil
	}

	// Release the flock, then close and clear the lock file.
	if err := unix.Flock(int(l.file.Fd()), unix.LOCK_UN); err != nil {
		return err
	}
	l.held = false
	err := l.file.Close()
	l.file = nil
	return err
}
