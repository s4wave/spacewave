package wazerofs

import (
	"context"
	"io"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"
	wazero_sys "github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/sys"
)

// File is a writeable fs.File bridge backed by syscall functions needed for ABI
// including WASI.
//
// Any unsupported method or parameter should return ENOSYS.
//
// # Errors
//
// All methods that can return an error return a Errno, which is zero
// on success.
//
// Restricting to Errno matches current WebAssembly host functions,
// which are constrained to well-known error codes. For example, WASI maps syscall
// errors to u32 numeric values.
//
// # Notes
//
//   - A writable filesystem abstraction is not yet implemented as of Go 1.20.
//     See https://github.com/golang/go/issues/45757
type File struct {
	ctx       context.Context
	handle    *unixfs.FSHandle
	flag      wazero_sys.Oflag
	offset    int64  // current read/write offset
	dirOffset uint64 // current directory reading offset
}

// NewFile constructs a new File backed by a UnixFS FSHandle.
func NewFile(ctx context.Context, handle *unixfs.FSHandle, flag wazero_sys.Oflag) *File {
	return &File{
		ctx:    ctx,
		handle: handle,
		flag:   flag,
	}
}

// Dev returns the device ID (Stat_t.Dev) of this file, zero if unknown or
// an error retrieving it.
//
// # Errors
//
// Possible errors are those from Stat, except ENOSYS should not
// be returned. Zero should be returned if there is no implementation.
//
// # Notes
//
//   - Implementations should cache this result.
//   - This combined with Ino can implement os.SameFile.
func (f *File) Dev() (uint64, wazero_sys.Errno) {
	return 0, 0
}

// Ino returns the serial number (Stat_t.Ino) of this file, zero if unknown
// or an error retrieving it.
//
// # Errors
//
// Possible errors are those from Stat, except ENOSYS should not
// be returned. Zero should be returned if there is no implementation.
//
// # Notes
//
//   - Implementations should cache this result.
//   - This combined with Dev can implement os.SameFile.
func (f *File) Ino() (sys.Inode, wazero_sys.Errno) {
	return 0, 0
}

// IsDir returns true if this file is a directory or an error there was an
// error retrieving this information.
//
// # Errors
//
// Possible errors are those from Stat, except ENOSYS should not
// be returned. false should be returned if there is no implementation.
//
// # Notes
//
//   - Implementations should cache this result.
func (f *File) IsDir() (bool, wazero_sys.Errno) {
	if f.handle == nil {
		return false, wazero_sys.EBADF
	}

	nodeType, err := f.handle.GetNodeType(f.ctx)
	if err != nil {
		return false, UnixfsErrorToWazeroErrno(err)
	}

	return nodeType.GetIsDirectory(), 0
}

// IsAppend returns true if the file was opened with O_APPEND, or
// SetAppend was successfully enabled on this file.
//
// # Notes
//
//   - This might not match the underlying state of the file descriptor if
//     the file was not opened via OpenFile.
func (f *File) IsAppend() bool {
	return f.flag&wazero_sys.O_APPEND != 0
}

// SetAppend toggles the append mode (O_APPEND) of this file.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - EBADF: the file or directory was closed.
//
// # Notes
//
//   - There is no `O_APPEND` for `fcntl` in POSIX, so implementations may
//     have to re-open the underlying file to apply this. See
//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/open.html
func (f *File) SetAppend(enable bool) wazero_sys.Errno {
	if f.handle == nil {
		return wazero_sys.EBADF
	}

	// Check if it's a directory
	nodeType, err := f.handle.GetNodeType(f.ctx)
	if err != nil {
		return UnixfsErrorToWazeroErrno(err)
	}

	if nodeType.GetIsDirectory() {
		return wazero_sys.EBADF
	}

	// Toggle the O_APPEND flag
	if enable {
		f.flag |= wazero_sys.O_APPEND
		// When enabling append mode, set offset to end of file
		size, err := f.handle.GetSize(f.ctx)
		if err != nil {
			return UnixfsErrorToWazeroErrno(err)
		}
		if size > 0x7FFFFFFFFFFFFFFF {
			return wazero_sys.EINVAL
		}
		f.offset = int64(size)
	} else {
		f.flag &^= wazero_sys.O_APPEND
	}

	return 0
}

// Stat is similar to syscall.Fstat.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - EBADF: the file or directory was closed.
//
// # Notes
//
//   - This is like syscall.Fstat and `fstatat` with `AT_FDCWD` in POSIX.
//     See https://pubs.opengroup.org/onlinepubs/9699919799/functions/stat.html
//   - A fs.FileInfo backed implementation sets atim, mtim and ctim to the
//     same value.
//   - Windows allows you to stat a closed directory.
func (f *File) Stat() (sys.Stat_t, wazero_sys.Errno) {
	if f.handle == nil {
		return sys.Stat_t{}, wazero_sys.EBADF
	}

	// Get file info to populate stat structure
	fileInfo, err := f.handle.GetFileInfo(f.ctx)
	if err != nil {
		return sys.Stat_t{}, UnixfsErrorToWazeroErrno(err)
	}

	// Convert file info to Stat_t
	return FileInfoToStat(fileInfo), 0
}

// Read attempts to read all bytes in the file into `buf`, and returns the
// count read even on error.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - EBADF: the file or directory was closed or not readable.
//   - EISDIR: the file was a directory.
//
// # Notes
//
//   - This is like io.Reader and `read` in POSIX, preferring semantics of
//     io.Reader. See https://pubs.opengroup.org/onlinepubs/9699919799/functions/read.html
//   - Unlike io.Reader, there is no io.EOF returned on end-of-file. To
//     read the file completely, the caller must repeat until `n` is zero.
func (f *File) Read(buf []byte) (n int, errno wazero_sys.Errno) {
	if f.handle == nil {
		return 0, wazero_sys.EBADF
	}

	// Check if file is readable
	if f.flag&wazero_sys.O_WRONLY != 0 {
		return 0, wazero_sys.EBADF
	}

	// Check if it's a directory
	nodeType, err := f.handle.GetNodeType(f.ctx)
	if err != nil {
		return 0, UnixfsErrorToWazeroErrno(err)
	}

	if nodeType.GetIsDirectory() {
		return 0, wazero_sys.EISDIR
	}

	// Read from the current offset
	bytesRead, err := f.handle.ReadAt(f.ctx, f.offset, buf)

	//   - Unlike io.Reader, there is no io.EOF returned on end-of-file. To
	//     read the file completely, the caller must repeat until `n` is zero.
	if err != nil && err != io.EOF {
		return int(bytesRead), UnixfsErrorToWazeroErrno(err)
	}

	// Update the offset
	f.offset += bytesRead

	return int(bytesRead), 0
}

// Pread attempts to read all bytes in the file into `p`, starting at the
// offset `off`, and returns the count read even on error.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - EBADF: the file or directory was closed or not readable.
//   - EINVAL: the offset was negative.
//   - EISDIR: the file was a directory.
//
// # Notes
//
//   - This is like io.ReaderAt and `pread` in POSIX, preferring semantics
//     of io.ReaderAt. See https://pubs.opengroup.org/onlinepubs/9699919799/functions/pread.html
//   - Unlike io.ReaderAt, there is no io.EOF returned on end-of-file. To
//     read the file completely, the caller must repeat until `n` is zero.
func (f *File) Pread(buf []byte, off int64) (n int, errno wazero_sys.Errno) {
	if f.handle == nil {
		return 0, wazero_sys.EBADF
	}

	// Check if offset is negative
	if off < 0 {
		return 0, wazero_sys.EINVAL
	}

	// Check if file is readable
	if f.flag&wazero_sys.O_WRONLY != 0 {
		return 0, wazero_sys.EBADF
	}

	// Check if it's a directory
	nodeType, err := f.handle.GetNodeType(f.ctx)
	if err != nil {
		return 0, UnixfsErrorToWazeroErrno(err)
	}

	if nodeType.GetIsDirectory() {
		return 0, wazero_sys.EISDIR
	}

	// Read from the specified offset without changing the file position
	bytesRead, err := f.handle.ReadAt(f.ctx, off, buf)
	if err != nil {
		return int(bytesRead), UnixfsErrorToWazeroErrno(err)
	}

	return int(bytesRead), 0
}

// Seek attempts to set the next offset for Read or Write and returns the
// resulting absolute offset or an error.
//
// # Parameters
//
// The `offset` parameters is interpreted in terms of `whence`:
//   - io.SeekStart: relative to the start of the file, e.g. offset=0 sets
//     the next Read or Write to the beginning of the file.
//   - io.SeekCurrent: relative to the current offset, e.g. offset=16 sets
//     the next Read or Write 16 bytes past the prior.
//   - io.SeekEnd: relative to the end of the file, e.g. offset=-1 sets the
//     next Read or Write to the last byte in the file.
//
// # Behavior when a directory
//
// The only supported use case for a directory is seeking to `offset` zero
// (`whence` = io.SeekStart). This should have the same behavior as
// os.File, which resets any internal state used by Readdir.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - EBADF: the file or directory was closed or not readable.
//   - EINVAL: the offset was negative.
//
// # Notes
//
//   - This is like io.Seeker and `fseek` in POSIX, preferring semantics
//     of io.Seeker. See https://pubs.opengroup.org/onlinepubs/9699919799/functions/fseek.html
func (f *File) Seek(offset int64, whence int) (newOffset int64, errno wazero_sys.Errno) {
	if f.handle == nil {
		return 0, wazero_sys.EBADF
	}

	// Check if it's a directory
	nodeType, err := f.handle.GetNodeType(f.ctx)
	if err != nil {
		return 0, UnixfsErrorToWazeroErrno(err)
	}

	if nodeType.GetIsDirectory() {
		// Only allow seeking to start for directories
		if whence == io.SeekStart && offset == 0 {
			f.offset = 0
			f.dirOffset = 0 // Reset directory reading position
			return 0, 0
		}
		return 0, wazero_sys.EINVAL
	}

	var newOff int64
	switch whence {
	case io.SeekStart:
		newOff = offset
	case io.SeekCurrent:
		newOff = f.offset + offset
	case io.SeekEnd:
		size, err := f.handle.GetSize(f.ctx)
		if err != nil {
			return 0, UnixfsErrorToWazeroErrno(err)
		}
		if size > 0x7FFFFFFFFFFFFFFF {
			return 0, wazero_sys.EINVAL
		}
		newOff = int64(size) + offset
	default:
		return 0, wazero_sys.EINVAL
	}

	if newOff < 0 {
		return 0, wazero_sys.EINVAL
	}

	f.offset = newOff
	return newOff, 0
}

// Readdir reads the contents of the directory associated with file and
// returns a slice of up to n Dirent values in an arbitrary order. This is
// a stateful function, so subsequent calls return any next values.
//
// If n > 0, Readdir returns at most n entries or an error.
// If n <= 0, Readdir returns all remaining entries or an error.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - EBADF: the file was closed or not a directory.
//   - ENOENT: the directory could not be read (e.g. deleted).
//
// # Notes
//
//   - This is like `Readdir` on os.File, but unlike `readdir` in POSIX.
//     See https://pubs.opengroup.org/onlinepubs/9699919799/functions/readdir.html
//   - Unlike os.File, there is no io.EOF returned on end-of-directory. To
//     read the directory completely, the caller must repeat until the
//     count read (`len(dirents)`) is less than `n`.
//   - See /RATIONALE.md for design notes.
func (f *File) Readdir(n int) (dirents []wazero_sys.Dirent, errno wazero_sys.Errno) {
	if f.handle == nil {
		return nil, wazero_sys.EBADF
	}

	// Check if it's a directory
	nodeType, err := f.handle.GetNodeType(f.ctx)
	if err != nil {
		return nil, UnixfsErrorToWazeroErrno(err)
	}

	if !nodeType.GetIsDirectory() {
		return nil, wazero_sys.EBADF
	}

	// Collect directory entries starting from current offset
	var entries []wazero_sys.Dirent
	var currentIndex uint64 = 0

	// Determine limit: if n <= 0, read all remaining entries; otherwise limit to n
	var limit uint64
	if n <= 0 {
		limit = 0 // 0 means no limit in ReaddirAll
	} else {
		limit = uint64(n)
	}

	err = f.handle.ReaddirAll(f.ctx, f.dirOffset, func(ent unixfs.FSCursorDirent) error {
		// If we have a limit and reached it, stop
		if limit > 0 && uint64(len(entries)) >= limit {
			return context.Canceled // Stop iteration
		}

		// Get file info to determine type
		entHandle, err := f.handle.Lookup(f.ctx, ent.GetName())
		if err != nil {
			return err // Skip this entry on error
		}
		defer entHandle.Release()

		fileInfo, err := entHandle.GetFileInfo(f.ctx)
		if err != nil {
			return err // Skip this entry on error
		}

		dirent := wazero_sys.Dirent{
			Ino:  0,
			Name: ent.GetName(),
			Type: fileInfo.Mode().Type(),
		}

		entries = append(entries, dirent)
		currentIndex++
		return nil
	})

	if err != nil && err != context.Canceled {
		return nil, UnixfsErrorToWazeroErrno(err)
	}

	// Update directory offset for next call
	f.dirOffset += currentIndex

	return entries, 0
}

// Write attempts to write all bytes in `p` to the file, and returns the
// count written even on error.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - EBADF: the file was closed, not writeable, or a directory.
//
// # Notes
//
//   - This is like io.Writer and `write` in POSIX, preferring semantics of
//     io.Writer. See https://pubs.opengroup.org/onlinepubs/9699919799/functions/write.html
func (f *File) Write(buf []byte) (n int, errno wazero_sys.Errno) {
	if f.handle == nil {
		return 0, wazero_sys.EBADF
	}

	// Check if file is writable
	if f.flag&wazero_sys.O_RDONLY != 0 {
		return 0, wazero_sys.EBADF
	}

	// Check if it's a directory
	nodeType, err := f.handle.GetNodeType(f.ctx)
	if err != nil {
		return 0, UnixfsErrorToWazeroErrno(err)
	}

	if nodeType.GetIsDirectory() {
		return 0, wazero_sys.EBADF
	}

	// If opened with O_APPEND, seek to end
	if f.flag&wazero_sys.O_APPEND != 0 {
		size, err := f.handle.GetSize(f.ctx)
		if err != nil {
			return 0, UnixfsErrorToWazeroErrno(err)
		}
		if size > 0x7FFFFFFFFFFFFFFF {
			return 0, wazero_sys.EINVAL
		}
		f.offset = int64(size)
	}

	// Write to the current offset
	err = f.handle.WriteAt(f.ctx, f.offset, buf, time.Now())
	if err != nil {
		return 0, UnixfsErrorToWazeroErrno(err)
	}

	// Update the offset
	f.offset += int64(len(buf))

	return len(buf), 0
}

// Pwrite attempts to write all bytes in `p` to the file at the given
// offset `off`, and returns the count written even on error.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - EBADF: the file or directory was closed or not writeable.
//   - EINVAL: the offset was negative.
//   - EISDIR: the file was a directory.
//
// # Notes
//
//   - This is like io.WriterAt and `pwrite` in POSIX, preferring semantics
//     of io.WriterAt. See https://pubs.opengroup.org/onlinepubs/9699919799/functions/pwrite.html
func (f *File) Pwrite(buf []byte, off int64) (n int, errno wazero_sys.Errno) {
	if f.handle == nil {
		return 0, wazero_sys.EBADF
	}

	// Check if offset is negative
	if off < 0 {
		return 0, wazero_sys.EINVAL
	}

	// Check if file is writable
	if f.flag&wazero_sys.O_RDONLY != 0 {
		return 0, wazero_sys.EBADF
	}

	// Check if it's a directory
	nodeType, err := f.handle.GetNodeType(f.ctx)
	if err != nil {
		return 0, UnixfsErrorToWazeroErrno(err)
	}

	if nodeType.GetIsDirectory() {
		return 0, wazero_sys.EISDIR
	}

	// Write to the specified offset without changing the file position
	err = f.handle.WriteAt(f.ctx, off, buf, time.Now())
	if err != nil {
		return 0, UnixfsErrorToWazeroErrno(err)
	}

	return len(buf), 0
}

// Truncate truncates a file to a specified length.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - EBADF: the file or directory was closed.
//   - EINVAL: the `size` is negative.
//   - EISDIR: the file was a directory.
//
// # Notes
//
//   - This is like syscall.Ftruncate and `ftruncate` in POSIX. See
//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/ftruncate.html
func (f *File) Truncate(size int64) wazero_sys.Errno {
	if f.handle == nil {
		return wazero_sys.EBADF
	}

	// Check if size is negative
	if size < 0 {
		return wazero_sys.EINVAL
	}

	// Check if it's a directory
	nodeType, err := f.handle.GetNodeType(f.ctx)
	if err != nil {
		return UnixfsErrorToWazeroErrno(err)
	}

	if nodeType.GetIsDirectory() {
		return wazero_sys.EISDIR
	}

	// Truncate the file to the specified size
	err = f.handle.Truncate(f.ctx, uint64(size), time.Now())
	if err != nil {
		return UnixfsErrorToWazeroErrno(err)
	}

	return 0
}

// Sync synchronizes changes to the file.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - EBADF: the file or directory was closed.
//
// # Notes
//
//   - This is like syscall.Fsync and `fsync` in POSIX. See
//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/fsync.html
//   - This returns with no error instead of ENOSYS when
//     unimplemented. This prevents fake filesystems from erring.
func (f *File) Sync() wazero_sys.Errno {
	return 0
}

// Datasync synchronizes the data of a file.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - EBADF: the file or directory was closed.
//
// # Notes
//
//   - This is like syscall.Fdatasync and `fdatasync` in POSIX. See
//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/fdatasync.html
//   - This returns with no error instead of ENOSYS when
//     unimplemented. This prevents fake filesystems from erring.
//   - As this is commonly missing, some implementations dispatch to Sync.
func (f *File) Datasync() wazero_sys.Errno {
	return 0
}

// Utimens set file access and modification times of this file, at
// nanosecond precision.
//
// # Parameters
//
// The `atim` and `mtim` parameters refer to access and modification time
// stamps as defined in sys.Stat_t. To retain one or the other, substitute
// it with the pseudo-timestamp UTIME_OMIT.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - EBADF: the file or directory was closed.
//
// # Notes
//
//   - This is like syscall.UtimesNano and `futimens` in POSIX. See
//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/futimens.html
//   - Access times are ignored as the underlying UnixFS filesystem does not
//     support separate access and modification times.
func (f *File) Utimens(atim, mtim int64) wazero_sys.Errno {
	if f.handle == nil {
		return wazero_sys.EBADF
	}

	// Only handle modification time updates, ignore access time
	if mtim == wazero_sys.UTIME_OMIT {
		return 0 // Nothing to do
	}

	// Convert nanoseconds to time.Time
	mtime := time.Unix(mtim/1e9, mtim%1e9)

	// Set the modification timestamp
	err := f.handle.SetModTimestamp(f.ctx, mtime)
	if err != nil {
		return UnixfsErrorToWazeroErrno(err)
	}

	return 0
}

// Close closes the underlying file.
//
// A zero Errno is returned if unimplemented or success.
//
// # Notes
//
//   - This is like syscall.Close and `close` in POSIX. See
//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/close.html
func (f *File) Close() wazero_sys.Errno {
	if f.handle != nil {
		f.handle.Release()
		f.handle = nil
	}
	return 0
}

// _ is a type assertion
var _ wazero_sys.File = ((*File)(nil))
