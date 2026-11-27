package checkoutroot

import (
	"context"
	"errors"
	"os"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

// openAnonymousStage opens an inode on the destination's own filesystem.
func openAnonymousStage(dir *os.Root) (*os.File, error) {
	// Open the anonymous inode relative to the pinned destination.
	parent, err := dir.Open(".")
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	fd, err := unix.Openat(int(parent.Fd()), ".", unix.O_TMPFILE|unix.O_RDWR|unix.O_CLOEXEC, 0o600)
	if anonymousStageUnsupported(err) {
		return nil, errors.ErrUnsupported
	}
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), "anonymous upload"), nil
}

// publishAnonymousStage installs a complete inode if its destination is absent.
func publishAnonymousStage(ctx context.Context, dir *os.Root, file *os.File, name string, ts time.Time) error {
	// Address the inode through the file's fd without requiring
	// CAP_DAC_READ_SEARCH.
	source := "/proc/self/fd/" + strconv.FormatUint(uint64(file.Fd()), 10)
	when := unix.Timespec{Sec: ts.Unix(), Nsec: int64(ts.Nanosecond())}
	if err := unix.UtimesNanoAt(unix.AT_FDCWD, source, []unix.Timespec{when, when}, 0); err != nil {
		if anonymousStageUnsupported(err) {
			return errors.ErrUnsupported
		}
		return err
	}

	// Link only after metadata and cancellation checks have succeeded.
	parent, err := dir.Open(".")
	if err != nil {
		return err
	}
	defer parent.Close()
	if err := ctx.Err(); err != nil {
		return err
	}
	err = unix.Linkat(unix.AT_FDCWD, source, int(parent.Fd()), name, unix.AT_SYMLINK_FOLLOW)
	if anonymousStageUnsupported(err) {
		return errors.ErrUnsupported
	}
	return err
}

// anonymousStageUnsupported recognizes missing temporary-inode or procfs support.
func anonymousStageUnsupported(err error) bool {
	return errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.EINVAL) ||
		errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EISDIR) ||
		errors.Is(err, unix.ENOENT) || errors.Is(err, unix.EPERM)
}
