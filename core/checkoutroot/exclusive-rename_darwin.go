package checkoutroot

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// renameExclusive publishes a named stage on a pinned macOS directory.
func renameExclusive(dir *os.Root, temp, name string) error {
	// Use the mount's native no-replace rename without re-resolving the root.
	fd, err := dir.Open(".")
	if err != nil {
		return err
	}
	defer fd.Close()
	err = unix.RenameatxNp(int(fd.Fd()), temp, int(fd.Fd()), name, unix.RENAME_EXCL)

	// Use a hard link on mounts without exclusive rename.
	if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.ENOTSUP) {
		return dir.Link(temp, name)
	}
	return err
}
