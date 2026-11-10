//go:build unix && !js && !wasip1

package packahead

import "golang.org/x/sys/unix"

// freeDiskBytes returns the space free to this process on the file system
// holding dir.
func freeDiskBytes(dir string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil //nolint:gosec,unconvert
}
