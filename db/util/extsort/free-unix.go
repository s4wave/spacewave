//go:build unix

package extsort

import "golang.org/x/sys/unix"

// freeBytes returns the bytes an unprivileged writer can use on dir's file
// system.
func freeBytes(dir string) (int64, error) {
	var fs unix.Statfs_t
	if err := unix.Statfs(dir, &fs); err != nil {
		return 0, err
	}
	return int64(fs.Bavail) * int64(fs.Bsize), nil //nolint:gosec,unconvert // block counts fit int64; Bsize's type varies by OS.
}
