//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd)

package logfile

import "os"

// markLogInUse is a no-op on platforms without flock. Windows refuses to
// remove open files, which protects active logs there.
func markLogInUse(_ *os.File) {}

// logInUse reports false on platforms without flock.
func logInUse(_ string) bool {
	return false
}
