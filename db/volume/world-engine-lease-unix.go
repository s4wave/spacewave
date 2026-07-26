//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly || solaris

package volume

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func tryLockWorldEngineLease(file *os.File) (bool, error) {
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func unlockWorldEngineLease(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
