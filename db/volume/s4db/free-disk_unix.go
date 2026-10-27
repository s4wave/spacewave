//go:build unix && !js && !wasip1

package volume_s4db

import (
	"path/filepath"

	"golang.org/x/sys/unix"
)

// freeDiskBytes returns the space free to this process on the file system
// holding the file at path.
func freeDiskBytes(path string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(filepath.Dir(path), &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil //nolint:gosec,unconvert
}
