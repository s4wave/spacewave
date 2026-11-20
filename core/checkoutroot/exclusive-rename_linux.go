package checkoutroot

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// renameExclusive publishes a named stage on a pinned Linux directory.
func renameExclusive(dir *os.Root, temp, name string) error {
	// Use renameat2 so a mount without hard links can publish a new file.
	fd, err := dir.Open(".")
	if err != nil {
		return err
	}
	defer fd.Close()
	err = unix.Renameat2(int(fd.Fd()), temp, int(fd.Fd()), name, unix.RENAME_NOREPLACE)

	// Use a hard link on filesystems that lack the exclusive rename flag.
	if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EOPNOTSUPP) {
		return dir.Link(temp, name)
	}
	return err
}
