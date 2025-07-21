package plugin_host_wazero_quickjs

import (
	"io"
	"io/fs"
	"time"

	wazero_exp_sys "github.com/tetratelabs/wazero/experimental/sys"
	wazero_sys "github.com/tetratelabs/wazero/sys"
)

// devFS implements a minimal /dev filesystem for QuickJS I/O operations.
// It provides /dev/out as a write-only file for async output.
type devFS struct {
	writer io.Writer
}

// newDevFS creates a new devFS with the given writer for /dev/out.
func newDevFS(writer io.Writer) *devFS {
	return &devFS{writer: writer}
}

// OpenFile opens a file in the /dev filesystem.
func (d *devFS) OpenFile(name string, flag wazero_exp_sys.Oflag, perm fs.FileMode) (wazero_exp_sys.File, wazero_exp_sys.Errno) {
	switch name {
	case ".":
		// /dev directory - read-only
		if flag&(wazero_exp_sys.O_WRONLY|wazero_exp_sys.O_RDWR) != 0 {
			return nil, wazero_exp_sys.EACCES
		}
		return &DevDirFile{}, 0
	case "out":
		return &DevOutFile{writer: d.writer}, 0
	default:
		return nil, wazero_exp_sys.ENOENT
	}
}

// Lstat returns file status for /dev files.
func (d *devFS) Lstat(name string) (wazero_sys.Stat_t, wazero_exp_sys.Errno) {
	switch name {
	case ".":
		return wazero_sys.Stat_t{
			Mode: fs.ModeDir | 0o555, // read-only directory
			Size: 0,
		}, 0
	case "out":
		return wazero_sys.Stat_t{
			Mode: fs.ModeCharDevice | 0o200, // write-only character device
			Size: 0,
		}, 0
	default:
		return wazero_sys.Stat_t{}, wazero_exp_sys.ENOENT
	}
}

// Stat returns file status for /dev files (same as Lstat for this implementation).
func (d *devFS) Stat(name string) (wazero_sys.Stat_t, wazero_exp_sys.Errno) {
	return d.Lstat(name)
}

// Mkdir is not supported in /dev.
func (d *devFS) Mkdir(name string, perm fs.FileMode) wazero_exp_sys.Errno {
	return wazero_exp_sys.ENOSYS
}

// Chmod is not supported in /dev.
func (d *devFS) Chmod(name string, perm fs.FileMode) wazero_exp_sys.Errno {
	return wazero_exp_sys.ENOSYS
}

// Rename is not supported in /dev.
func (d *devFS) Rename(from, to string) wazero_exp_sys.Errno {
	return wazero_exp_sys.ENOSYS
}

// Rmdir is not supported in /dev.
func (d *devFS) Rmdir(name string) wazero_exp_sys.Errno {
	return wazero_exp_sys.ENOSYS
}

// Unlink is not supported in /dev.
func (d *devFS) Unlink(name string) wazero_exp_sys.Errno {
	return wazero_exp_sys.ENOSYS
}

// Link is not supported in /dev.
func (d *devFS) Link(oldname, newname string) wazero_exp_sys.Errno {
	return wazero_exp_sys.ENOSYS
}

// Symlink is not supported in /dev.
func (d *devFS) Symlink(oldname, linkname string) wazero_exp_sys.Errno {
	return wazero_exp_sys.ENOSYS
}

// Readlink is not supported in /dev.
func (d *devFS) Readlink(name string) (string, wazero_exp_sys.Errno) {
	return "", wazero_exp_sys.ENOSYS
}

// Utimens is not supported in /dev.
func (d *devFS) Utimens(name string, atim, mtim int64) wazero_exp_sys.Errno {
	return wazero_exp_sys.ENOSYS
}

// _ is a type assertion
var _ wazero_exp_sys.FS = ((*devFS)(nil))

// DevDirFile implements a read-only directory for /dev.
type DevDirFile struct{}

// Dev returns 0 for device ID.
func (f *DevDirFile) Dev() (uint64, wazero_exp_sys.Errno) {
	return 0, 0
}

// Ino returns 0 for inode number.
func (f *DevDirFile) Ino() (wazero_sys.Inode, wazero_exp_sys.Errno) {
	return 0, 0
}

// IsDir returns true as this is a directory.
func (f *DevDirFile) IsDir() (bool, wazero_exp_sys.Errno) {
	return true, 0
}

// IsAppend returns false as directories don't support append mode.
func (f *DevDirFile) IsAppend() bool {
	return false
}

// SetAppend is not supported for directories.
func (f *DevDirFile) SetAppend(enable bool) wazero_exp_sys.Errno {
	return wazero_exp_sys.ENOSYS
}

// Stat returns file status for /dev directory.
func (f *DevDirFile) Stat() (wazero_sys.Stat_t, wazero_exp_sys.Errno) {
	now := time.Now().UnixNano()
	return wazero_sys.Stat_t{
		Mode: fs.ModeDir | 0o555, // read-only directory
		Size: 0,
		Mtim: now,
		Atim: now,
		Ctim: now,
	}, 0
}

// Read is not supported on directories.
func (f *DevDirFile) Read(buf []byte) (int, wazero_exp_sys.Errno) {
	return 0, wazero_exp_sys.EISDIR
}

// Pread is not supported on directories.
func (f *DevDirFile) Pread(buf []byte, off int64) (int, wazero_exp_sys.Errno) {
	return 0, wazero_exp_sys.EISDIR
}

// Seek is not supported on directories.
func (f *DevDirFile) Seek(offset int64, whence int) (int64, wazero_exp_sys.Errno) {
	return 0, wazero_exp_sys.ENOSYS
}

// Readdir returns directory entries for /dev.
func (f *DevDirFile) Readdir(n int) ([]wazero_exp_sys.Dirent, wazero_exp_sys.Errno) {
	entries := []wazero_exp_sys.Dirent{
		{Name: "out", Type: fs.ModeCharDevice},
	}

	if n <= 0 {
		return entries, 0
	}

	if n > len(entries) {
		n = len(entries)
	}

	return entries[:n], 0
}

// Write is not supported on directories.
func (f *DevDirFile) Write(buf []byte) (int, wazero_exp_sys.Errno) {
	return 0, wazero_exp_sys.EISDIR
}

// Pwrite is not supported on directories.
func (f *DevDirFile) Pwrite(buf []byte, off int64) (int, wazero_exp_sys.Errno) {
	return 0, wazero_exp_sys.EISDIR
}

// Truncate is not supported on directories.
func (f *DevDirFile) Truncate(size int64) wazero_exp_sys.Errno {
	return wazero_exp_sys.EISDIR
}

// Sync is a no-op for directories.
func (f *DevDirFile) Sync() wazero_exp_sys.Errno {
	return 0
}

// Datasync is a no-op for directories.
func (f *DevDirFile) Datasync() wazero_exp_sys.Errno {
	return 0
}

// Utimens is not supported on directories.
func (f *DevDirFile) Utimens(atim, mtim int64) wazero_exp_sys.Errno {
	return wazero_exp_sys.ENOSYS
}

// Close is a no-op for directories.
func (f *DevDirFile) Close() wazero_exp_sys.Errno {
	return 0
}

// DevOutFile implements a write-only file for /dev/out.
type DevOutFile struct {
	writer io.Writer
}

// Dev returns 0 for device ID.
func (f *DevOutFile) Dev() (uint64, wazero_exp_sys.Errno) {
	return 0, 0
}

// Ino returns 0 for inode number.
func (f *DevOutFile) Ino() (wazero_sys.Inode, wazero_exp_sys.Errno) {
	return 0, 0
}

// IsDir returns false as /dev/out is not a directory.
func (f *DevOutFile) IsDir() (bool, wazero_exp_sys.Errno) {
	return false, 0
}

// IsAppend returns false as /dev/out doesn't support append mode.
func (f *DevOutFile) IsAppend() bool {
	return false
}

// SetAppend is not supported for /dev/out.
func (f *DevOutFile) SetAppend(enable bool) wazero_exp_sys.Errno {
	return wazero_exp_sys.ENOSYS
}

// Stat returns file status for /dev/out.
func (f *DevOutFile) Stat() (wazero_sys.Stat_t, wazero_exp_sys.Errno) {
	now := time.Now().UnixNano()
	return wazero_sys.Stat_t{
		Mode: fs.ModeCharDevice | 0o200, // write-only character device
		Size: 0,
		Mtim: now,
		Atim: now,
		Ctim: now,
	}, 0
}

// Read is not supported on /dev/out (write-only).
func (f *DevOutFile) Read(buf []byte) (int, wazero_exp_sys.Errno) {
	return 0, wazero_exp_sys.EBADF
}

// Pread is not supported on /dev/out (write-only).
func (f *DevOutFile) Pread(buf []byte, off int64) (int, wazero_exp_sys.Errno) {
	return 0, wazero_exp_sys.EBADF
}

// Seek is not supported on /dev/out.
func (f *DevOutFile) Seek(offset int64, whence int) (int64, wazero_exp_sys.Errno) {
	return 0, wazero_exp_sys.ENOSYS
}

// Readdir is not supported on /dev/out (not a directory).
func (f *DevOutFile) Readdir(n int) ([]wazero_exp_sys.Dirent, wazero_exp_sys.Errno) {
	return nil, wazero_exp_sys.ENOTDIR
}

// Write writes data to the underlying writer.
func (f *DevOutFile) Write(buf []byte) (int, wazero_exp_sys.Errno) {
	n, err := f.writer.Write(buf)
	if err != nil {
		return n, wazero_exp_sys.EIO
	}
	return n, 0
}

// Pwrite is not supported on /dev/out.
func (f *DevOutFile) Pwrite(buf []byte, off int64) (int, wazero_exp_sys.Errno) {
	return 0, wazero_exp_sys.ENOSYS
}

// Truncate is not supported on /dev/out.
func (f *DevOutFile) Truncate(size int64) wazero_exp_sys.Errno {
	return wazero_exp_sys.ENOSYS
}

// Sync is a no-op for /dev/out.
func (f *DevOutFile) Sync() wazero_exp_sys.Errno {
	return 0
}

// Datasync is a no-op for /dev/out.
func (f *DevOutFile) Datasync() wazero_exp_sys.Errno {
	return 0
}

// Utimens is not supported on /dev/out.
func (f *DevOutFile) Utimens(atim, mtim int64) wazero_exp_sys.Errno {
	return wazero_exp_sys.ENOSYS
}

// Close is a no-op for /dev/out.
func (f *DevOutFile) Close() wazero_exp_sys.Errno {
	return 0
}

// _ is a type assertion
var (
	_ wazero_exp_sys.FS   = (*devFS)(nil)
	_ wazero_exp_sys.File = (*DevDirFile)(nil)
	_ wazero_exp_sys.File = (*DevOutFile)(nil)
)
