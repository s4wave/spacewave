//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package logfile

import (
	"errors"
	"os"
	"syscall"
)

// markLogInUse takes a shared advisory lock on an open log file so that
// PruneOldLogs in any process skips it. Closing f releases the lock.
func markLogInUse(f *os.File) {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
}

// logInUse reports whether a writer holds the advisory lock on path.
func logInUse(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	return errors.Is(err, syscall.EWOULDBLOCK)
}
