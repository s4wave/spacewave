//go:build unix

package diskfree

import "golang.org/x/sys/unix"

// Bytes returns the bytes an unprivileged writer can use on the file system
// holding dir.
func Bytes(dir string) (uint64, error) {
	var fs unix.Statfs_t
	if err := unix.Statfs(dir, &fs); err != nil {
		return 0, err
	}
	return uint64(fs.Bavail) * uint64(fs.Bsize), nil //nolint:gosec,unconvert // Bsize's type varies by OS.
}
