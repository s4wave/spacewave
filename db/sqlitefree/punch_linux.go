//go:build linux

package sqlitefree

import (
	"os"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// Supported reports whether PunchRanges can release blocks on this system.
const Supported = true

// PunchRanges releases each range's blocks, keeping the file size.
func PunchRanges(f *os.File, pageSize int64, rs []Range) error {
	fd := int(f.Fd())
	for _, r := range rs {
		off := int64(r.Start-1) * pageSize
		n := int64(r.Count) * pageSize
		if err := unix.Fallocate(fd, unix.FALLOC_FL_PUNCH_HOLE|unix.FALLOC_FL_KEEP_SIZE, off, n); err != nil {
			return errors.Wrapf(err, "pages %d+%d", r.Start, r.Count)
		}
	}
	return nil
}

// IsHole reports whether range r of a file of pages of pageSize holds no
// data blocks, as SEEK_DATA finds them: a range punched before reads as a
// hole. A file system without SEEK_DATA reports no hole.
func IsHole(f *os.File, pageSize int64, r Range) bool {
	off := int64(r.Start-1) * pageSize
	data, err := unix.Seek(int(f.Fd()), off, unix.SEEK_DATA)
	if errors.Is(err, unix.ENXIO) {
		return true
	}
	return err == nil && data >= off+int64(r.Count)*pageSize
}
