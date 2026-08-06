//go:build unix

package npm

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func (l *installLock) tryLock() (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held {
		return true, nil
	}
	if l.file == nil {
		file, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDONLY, 0o600)
		if err != nil {
			return false, err
		}
		l.file = file
	}
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
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.held || l.file == nil {
		return nil
	}
	if err := unix.Flock(int(l.file.Fd()), unix.LOCK_UN); err != nil {
		return err
	}
	l.held = false
	err := l.file.Close()
	l.file = nil
	return err
}
