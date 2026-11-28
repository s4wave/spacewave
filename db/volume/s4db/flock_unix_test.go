//go:build !js && !wasip1 && !windows

package volume_s4db_test

import (
	"errors"
	"os"
	"syscall"
)

// boltLocked reports whether another open file holds an exclusive flock on
// the file f names, by trying a shared one without waiting.
func boltLocked(f *os.File) (bool, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
