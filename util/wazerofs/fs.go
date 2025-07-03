// Package wazerofs provides a simplified filesystem adapter that bridges UnixFS
// with the Wazero WebAssembly runtime's filesystem interface.
//
// This implementation focuses on basic file operations and intentionally omits
// advanced FUSE-like features such as inode IDs, device IDs, and complex
// filesystem metadata. While these features are fully implemented in the
// unixfs fuse package, they add significant complexity and are not currently
// needed for WebAssembly workloads running in Wazero. This simplified approach
// provides the essential filesystem operations required for most use cases.
package wazerofs

import (
	"context"
	"io/fs"
	"time"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	wazero_exp_sys "github.com/tetratelabs/wazero/experimental/sys"
	wazero_sysfs "github.com/tetratelabs/wazero/experimental/sys"
	wazero_sys "github.com/tetratelabs/wazero/sys"
)

// var sys.FS = ...
// config := wazero.NewFSConfig()
// config.(sysfs.FSConfig).WithSysFSMount(myFs, guestPath)
// https://github.com/tetratelabs/wazero/issues/2076

// FS implements a UnixFS FSHandle passed to the wazero vm.
type FS struct {
	ctx     context.Context
	fsh     *unixfs.FSHandle
	workdir []string
}

// NewFS constructs a new UnixFS FSHandle adapted to wazero FS.
//
// ctx is used for all fs operations
// workdir is prepended to non-absolute file paths
func NewFS(ctx context.Context, fsh *unixfs.FSHandle, workdir []string) *FS {
	return &FS{ctx: ctx, fsh: fsh, workdir: workdir}
}

// resolvePath resolves a path string to absolute path components, handling workdir prefix.
// Returns the resolved path components and any validation error.
func (f *FS) resolvePath(path string) ([]string, wazero_exp_sys.Errno) {
	pathPts, isAbsolute := unixfs.SplitPath(path)

	if !isAbsolute && len(f.workdir) != 0 {
		nparts := make([]string, len(pathPts)+len(f.workdir))
		copy(nparts, f.workdir)
		copy(nparts[len(f.workdir):], pathPts)
		pathPts = nparts
	}

	return pathPts, 0
}

// FileInfoToStat converts fs.FileInfo to wazero_sys.Stat_t.
// Sets atim, mtim and ctim to the same value as per the interface documentation.
func FileInfoToStat(fileInfo fs.FileInfo) wazero_sys.Stat_t {
	modTime := fileInfo.ModTime().UnixNano()
	return wazero_sys.Stat_t{
		Size: fileInfo.Size(),
		Mode: fileInfo.Mode(),
		Mtim: modTime,
		Atim: modTime, // Use same time for access time
		Ctim: modTime, // Use same time for change time
	}
}

// OpenFile opens a file. It should be closed via Close on File.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - EINVAL: `path` or `flag` is invalid.
//   - EISDIR: the path was a directory, but flag included O_RDWR or
//     O_WRONLY
//   - ENOENT: `path` doesn't exist and `flag` doesn't contain O_CREAT.
//
// # Constraints on the returned file
//
// Implementations that can read flags should enforce them regardless of
// the type returned. For example, while os.File implements io.Writer,
// attempts to write to a directory or a file opened with O_RDONLY fail
// with a EBADF.
//
// Some implementations choose whether to enforce read-only opens, namely
// fs.FS. While fs.FS is supported (Adapt), wazero cannot runtime enforce
// open flags. Instead, we encourage good behavior and test our built-in
// implementations.
//
// # Notes
//
//   - This is like os.OpenFile, except the path is relative to this file
//     system, and Errno is returned instead of os.PathError.
//   - Implications of permissions when O_CREAT are described in Chmod notes.
//   - This is like `open` in POSIX. See
//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/open.html
func (f *FS) OpenFile(openPath string, flag wazero_exp_sys.Oflag, perm fs.FileMode) (wazero_exp_sys.File, wazero_exp_sys.Errno) {
	pathPts, errno := f.resolvePath(openPath)
	if errno != 0 {
		return nil, errno
	}

	// Handle empty path case (current directory)
	if len(pathPts) == 0 {
		// Check if flags allow directory access
		if flag&wazero_exp_sys.O_RDWR != 0 || flag&wazero_exp_sys.O_WRONLY != 0 {
			return nil, wazero_exp_sys.EISDIR
		}
		// Open the current directory (root of filesystem)
		rootFsh, err := f.fsh.Clone(f.ctx)
		if err != nil {
			return nil, UnixfsErrorToWazeroErrno(err)
		}
		return NewFile(f.ctx, rootFsh, flag), 0
	}

	// get the filesystem handle to the containing dir
	dirFsh := f.fsh
	if len(pathPts) > 1 {
		var err error
		dirFsh, _, err = f.fsh.LookupPathPts(f.ctx, pathPts[:len(pathPts)-1])
		if err != nil {
			if dirFsh != nil {
				dirFsh.Release()
			}
			return nil, UnixfsErrorToWazeroErrno(err)
		}
		defer dirFsh.Release()
	}

	// get the filename from the path
	filename := pathPts[len(pathPts)-1]

	// lookup the file in the directory
	fileFsh, err := dirFsh.Lookup(f.ctx, filename)
	if err != nil {
		// if file doesn't exist and O_CREAT is set, create it
		if flag&wazero_exp_sys.O_CREAT != 0 && err == unixfs_errors.ErrNotExist {
			// create the file
			err = dirFsh.Mknod(f.ctx, true, []string{filename}, unixfs.NewFSCursorNodeType_File(), perm, time.Now())
			if err != nil {
				return nil, UnixfsErrorToWazeroErrno(err)
			}
			// lookup the newly created file
			fileFsh, err = dirFsh.Lookup(f.ctx, filename)
			if err != nil {
				return nil, UnixfsErrorToWazeroErrno(err)
			}
		} else {
			return nil, UnixfsErrorToWazeroErrno(err)
		}
	}

	// check if it's a directory and flags don't allow directory access
	nodeType, err := fileFsh.GetNodeType(f.ctx)
	if err != nil {
		fileFsh.Release()
		return nil, UnixfsErrorToWazeroErrno(err)
	}

	if nodeType.GetIsDirectory() && (flag&wazero_exp_sys.O_RDWR != 0 || flag&wazero_exp_sys.O_WRONLY != 0) {
		fileFsh.Release()
		return nil, wazero_exp_sys.EISDIR
	}

	// truncate file if O_TRUNC is set
	if flag&wazero_exp_sys.O_TRUNC != 0 && nodeType.GetIsFile() {
		err = fileFsh.Truncate(f.ctx, 0, time.Now())
		if err != nil {
			fileFsh.Release()
			return nil, UnixfsErrorToWazeroErrno(err)
		}
	}

	// create and return the file wrapper
	file := NewFile(f.ctx, fileFsh, flag)
	return file, 0
}

// Lstat gets file status without following symbolic links.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - ENOENT: `path` doesn't exist.
//
// # Notes
//
//   - This is like syscall.Lstat, except the `path` is relative to this
//     file system.
//   - This is like `lstat` in POSIX. See
//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/lstat.html
//   - An fs.FileInfo backed implementation sets atim, mtim and ctim to the
//     same value.
//   - When the path is a symbolic link, the stat returned is for the link,
//     not the file it refers to.
func (f *FS) Lstat(path string) (wazero_sys.Stat_t, wazero_exp_sys.Errno) {
	pathPts, errno := f.resolvePath(path)
	if errno != 0 {
		return wazero_sys.Stat_t{}, errno
	}

	// Handle empty path case (current directory)
	if len(pathPts) == 0 {
		// Return stats for the current directory (root of this filesystem)
		fileInfo, err := f.fsh.GetFileInfo(f.ctx)
		if err != nil {
			return wazero_sys.Stat_t{}, UnixfsErrorToWazeroErrno(err)
		}
		return FileInfoToStat(fileInfo), 0
	}

	// Look up the file/directory handle without following symlinks
	// We need to traverse to the parent and then lookup the final component
	var fileFsh *unixfs.FSHandle
	var err error

	if len(pathPts) == 1 {
		// Looking up in root directory
		fileFsh, err = f.fsh.Lookup(f.ctx, pathPts[0])
		if err != nil {
			return wazero_sys.Stat_t{}, UnixfsErrorToWazeroErrno(err)
		}
	} else {
		// Get parent directory first
		parentFsh, _, err := f.fsh.LookupPathPts(f.ctx, pathPts[:len(pathPts)-1])
		if err != nil {
			if parentFsh != nil {
				parentFsh.Release()
			}
			return wazero_sys.Stat_t{}, UnixfsErrorToWazeroErrno(err)
		}
		defer parentFsh.Release()

		// Look up the final component without following symlinks
		fileFsh, err = parentFsh.Lookup(f.ctx, pathPts[len(pathPts)-1])
		if err != nil {
			return wazero_sys.Stat_t{}, UnixfsErrorToWazeroErrno(err)
		}
	}
	defer fileFsh.Release()

	// Get file info to populate stat structure
	fileInfo, err := fileFsh.GetFileInfo(f.ctx)
	if err != nil {
		return wazero_sys.Stat_t{}, UnixfsErrorToWazeroErrno(err)
	}

	// Convert file info to Stat_t
	return FileInfoToStat(fileInfo), 0
}

// Stat gets file status.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - ENOENT: `path` doesn't exist.
//
// # Notes
//
//   - This is like syscall.Stat, except the `path` is relative to this
//     file system.
//   - This is like `stat` in POSIX. See
//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/stat.html
//   - An fs.FileInfo backed implementation sets atim, mtim and ctim to the
//     same value.
//   - When the path is a symbolic link, the stat returned is for the file
//     it refers to.
func (f *FS) Stat(path string) (wazero_sys.Stat_t, wazero_exp_sys.Errno) {
	pathPts, errno := f.resolvePath(path)
	if errno != 0 {
		return wazero_sys.Stat_t{}, errno
	}

	// Look up the file/directory handle, following symlinks
	fileFsh, _, err := f.fsh.LookupPathPts(f.ctx, pathPts)
	if err != nil {
		if fileFsh != nil {
			fileFsh.Release()
		}
		return wazero_sys.Stat_t{}, UnixfsErrorToWazeroErrno(err)
	}
	defer fileFsh.Release()

	// Get file info to populate stat structure
	fileInfo, err := fileFsh.GetFileInfo(f.ctx)
	if err != nil {
		return wazero_sys.Stat_t{}, UnixfsErrorToWazeroErrno(err)
	}

	// Convert file info to Stat_t
	return FileInfoToStat(fileInfo), 0
}

// Mkdir makes a directory.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - EINVAL: `path` is invalid.
//   - EEXIST: `path` exists and is a directory.
//   - ENOTDIR: `path` exists and is a file.
//
// # Notes
//
//   - This is like syscall.Mkdir, except the `path` is relative to this
//     file system.
//   - This is like `mkdir` in POSIX. See
//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/mkdir.html
//   - Implications of permissions are described in Chmod notes.
func (f *FS) Mkdir(path string, perm fs.FileMode) wazero_exp_sys.Errno {
	pathPts, errno := f.resolvePath(path)
	if errno != 0 {
		return errno
	}

	// Handle empty path case (current directory)
	if len(pathPts) == 0 {
		return wazero_exp_sys.EEXIST
	}

	// Get the parent directory handle
	var parentFsh *unixfs.FSHandle
	var dirname string
	var err error

	if len(pathPts) == 1 {
		// Creating in root
		parentFsh = f.fsh
		dirname = pathPts[0]
	} else {
		// Get parent directory
		parentFsh, _, err = f.fsh.LookupPathPts(f.ctx, pathPts[:len(pathPts)-1])
		if err != nil {
			if parentFsh != nil {
				parentFsh.Release()
			}
			return UnixfsErrorToWazeroErrno(err)
		}
		defer parentFsh.Release()
		dirname = pathPts[len(pathPts)-1]
	}

	// Create the directory
	err = parentFsh.Mknod(f.ctx, true, []string{dirname}, unixfs.NewFSCursorNodeType_Dir(), perm, time.Now())
	if err != nil {
		return UnixfsErrorToWazeroErrno(err)
	}

	return 0
}

// Chmod changes the mode of the file.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - EINVAL: `path` is invalid.
//   - ENOENT: `path` does not exist.
//
// # Notes
//
//   - This is like syscall.Chmod, except the `path` is relative to this
//     file system.
//   - This is like `chmod` in POSIX. See
//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/chmod.html
func (f *FS) Chmod(path string, perm fs.FileMode) wazero_exp_sys.Errno {
	pathPts, errno := f.resolvePath(path)
	if errno != 0 {
		return errno
	}

	// Look up the file/directory handle
	fileFsh, _, err := f.fsh.LookupPathPts(f.ctx, pathPts)
	if err != nil {
		if fileFsh != nil {
			fileFsh.Release()
		}
		return UnixfsErrorToWazeroErrno(err)
	}
	defer fileFsh.Release()

	// Set the permissions using the current time
	err = fileFsh.SetPermissions(f.ctx, perm, time.Now())
	if err != nil {
		return UnixfsErrorToWazeroErrno(err)
	}

	return 0
}

// Rename renames file or directory.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - EINVAL: `from` or `to` is invalid.
//   - ENOENT: `from` or `to` don't exist.
//   - ENOTDIR: `from` is a directory and `to` exists as a file.
//   - EISDIR: `from` is a file and `to` exists as a directory.
//   - ENOTEMPTY: `both from` and `to` are existing directory, but
//     `to` is not empty.
//
// # Notes
//
//   - This is like syscall.Rename, except the paths are relative to this
//     file system.
//   - This is like `rename` in POSIX. See
//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/rename.html
func (f *FS) Rename(from, to string) wazero_exp_sys.Errno {
	fromPathPts, errno := f.resolvePath(from)
	if errno != 0 {
		return errno
	}

	toPathPts, errno := f.resolvePath(to)
	if errno != 0 {
		return errno
	}

	// Handle empty path cases (current directory)
	if len(fromPathPts) == 0 || len(toPathPts) == 0 {
		return wazero_exp_sys.EPERM
	}

	// Look up the source file/directory handle
	fromFsh, _, err := f.fsh.LookupPathPts(f.ctx, fromPathPts)
	if err != nil {
		if fromFsh != nil {
			fromFsh.Release()
		}
		return UnixfsErrorToWazeroErrno(err)
	}
	defer fromFsh.Release()

	// Get the destination parent directory handle
	var toParentFsh *unixfs.FSHandle
	var toFilename string

	if len(toPathPts) == 1 {
		// Renaming to root
		toParentFsh = f.fsh
		toFilename = toPathPts[0]
	} else {
		// Get destination parent directory
		toParentFsh, _, err = f.fsh.LookupPathPts(f.ctx, toPathPts[:len(toPathPts)-1])
		if err != nil {
			if toParentFsh != nil {
				toParentFsh.Release()
			}
			return UnixfsErrorToWazeroErrno(err)
		}
		defer toParentFsh.Release()
		toFilename = toPathPts[len(toPathPts)-1]
	}

	// Perform the rename operation
	err = fromFsh.Rename(f.ctx, toParentFsh, toFilename, time.Now())
	if err != nil {
		return UnixfsErrorToWazeroErrno(err)
	}

	return 0
}

// Rmdir removes a directory.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - EINVAL: `path` is invalid.
//   - ENOENT: `path` doesn't exist.
//   - ENOTDIR: `path` exists, but isn't a directory.
//   - ENOTEMPTY: `path` exists, but isn't empty.
//
// # Notes
//
//   - This is like syscall.Rmdir, except the `path` is relative to this
//     file system.
//   - This is like `rmdir` in POSIX. See
//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/rmdir.html
func (f *FS) Rmdir(path string) wazero_exp_sys.Errno {
	pathPts, errno := f.resolvePath(path)
	if errno != 0 {
		return errno
	}

	// Handle root directory case (current directory)
	if len(pathPts) == 0 {
		return wazero_exp_sys.EPERM
	}

	// Get the parent directory handle
	var parentFsh *unixfs.FSHandle
	var filename string
	var err error

	if len(pathPts) == 1 {
		// Removing from root
		parentFsh = f.fsh
		filename = pathPts[0]
	} else {
		// Get parent directory
		parentFsh, _, err = f.fsh.LookupPathPts(f.ctx, pathPts[:len(pathPts)-1])
		if err != nil {
			if parentFsh != nil {
				parentFsh.Release()
			}
			return UnixfsErrorToWazeroErrno(err)
		}
		defer parentFsh.Release()
		filename = pathPts[len(pathPts)-1]
	}

	// First check if the target exists and is a directory
	targetFsh, err := parentFsh.Lookup(f.ctx, filename)
	if err != nil {
		return UnixfsErrorToWazeroErrno(err)
	}
	defer targetFsh.Release()

	// Check if it's a directory
	nodeType, err := targetFsh.GetNodeType(f.ctx)
	if err != nil {
		return UnixfsErrorToWazeroErrno(err)
	}

	if !nodeType.GetIsDirectory() {
		return wazero_exp_sys.ENOTDIR
	}

	// Check if directory is empty by trying to read entries
	var hasEntries bool
	err = targetFsh.ReaddirAll(f.ctx, 0, func(ent unixfs.FSCursorDirent) error {
		hasEntries = true
		return context.Canceled // Stop iteration early
	})
	if err != nil && err != context.Canceled {
		return UnixfsErrorToWazeroErrno(err)
	}

	if hasEntries {
		return wazero_exp_sys.ENOTEMPTY
	}

	// Remove the directory
	err = parentFsh.Remove(f.ctx, []string{filename}, time.Now())
	if err != nil {
		return UnixfsErrorToWazeroErrno(err)
	}

	return 0
}

// Unlink removes a directory entry.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - EINVAL: `path` is invalid.
//   - ENOENT: `path` doesn't exist.
//   - EISDIR: `path` exists, but is a directory.
//
// # Notes
//
//   - This is like syscall.Unlink, except the `path` is relative to this
//     file system.
//   - This is like `unlink` in POSIX. See
//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/unlink.html
func (f *FS) Unlink(path string) wazero_exp_sys.Errno {
	pathPts, errno := f.resolvePath(path)
	if errno != 0 {
		return errno
	}

	// Handle empty path case (current directory)
	if len(pathPts) == 0 {
		return wazero_exp_sys.EISDIR
	}

	// Get the parent directory handle
	var parentFsh *unixfs.FSHandle
	var filename string
	var err error

	if len(pathPts) == 1 {
		// Removing from root
		parentFsh = f.fsh
		filename = pathPts[0]
	} else {
		// Get parent directory
		parentFsh, _, err = f.fsh.LookupPathPts(f.ctx, pathPts[:len(pathPts)-1])
		if err != nil {
			if parentFsh != nil {
				parentFsh.Release()
			}
			return UnixfsErrorToWazeroErrno(err)
		}
		defer parentFsh.Release()
		filename = pathPts[len(pathPts)-1]
	}

	// First check if the target exists and is not a directory
	targetFsh, err := parentFsh.Lookup(f.ctx, filename)
	if err != nil {
		return UnixfsErrorToWazeroErrno(err)
	}
	defer targetFsh.Release()

	// Check if it's a directory
	nodeType, err := targetFsh.GetNodeType(f.ctx)
	if err != nil {
		return UnixfsErrorToWazeroErrno(err)
	}

	if nodeType.GetIsDirectory() {
		return wazero_exp_sys.EISDIR
	}

	// Remove the file/symlink
	err = parentFsh.Remove(f.ctx, []string{filename}, time.Now())
	if err != nil {
		return UnixfsErrorToWazeroErrno(err)
	}

	return 0
}

// Link creates a "hard" link from oldPath to newPath, in contrast to a
// soft link (via Symlink).
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - EPERM: `oldPath` is invalid.
//   - ENOENT: `oldPath` doesn't exist.
//   - EISDIR: `newPath` exists, but is a directory.
//
// # Notes
//
//   - This is like syscall.Link, except the `oldPath` is relative to this
//     file system.
//   - This is like `link` in POSIX. See
//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/link.html
//   - Hard links are not supported by the underlying UnixFS filesystem.
func (f *FS) Link(oldPath, newPath string) wazero_exp_sys.Errno {
	return wazero_exp_sys.ENOSYS
}

// Symlink creates a "soft" link from oldPath to newPath, in contrast to a
// hard link (via Link).
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - EPERM: `oldPath` or `newPath` is invalid.
//   - EEXIST: `newPath` exists.
//
// # Notes
//
//   - This is like syscall.Symlink, except the `oldPath` is relative to
//     this file system.
//   - This is like `symlink` in POSIX. See
//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/symlink.html
//   - Only `newPath` is relative to this file system and `oldPath` is kept
//     as-is. That is because the link is only resolved relative to the
//     directory when dereferencing it (e.g. ReadLink).
//     See https://github.com/bytecodealliance/cap-std/blob/v1.0.4/cap-std/src/fs/dir.rs#L404-L409
//     for how others implement this.
func (f *FS) Symlink(oldPath, linkName string) wazero_exp_sys.Errno {
	linkPathPts, errno := f.resolvePath(linkName)
	if errno != 0 {
		return errno
	}

	// Handle empty path case (current directory)
	if len(linkPathPts) == 0 {
		return wazero_exp_sys.EEXIST
	}

	// Get the parent directory handle
	var parentFsh *unixfs.FSHandle
	var filename string
	var err error

	if len(linkPathPts) == 1 {
		// Creating in root
		parentFsh = f.fsh
		filename = linkPathPts[0]
	} else {
		// Get parent directory
		parentFsh, _, err = f.fsh.LookupPathPts(f.ctx, linkPathPts[:len(linkPathPts)-1])
		if err != nil {
			if parentFsh != nil {
				parentFsh.Release()
			}
			return UnixfsErrorToWazeroErrno(err)
		}
		defer parentFsh.Release()
		filename = linkPathPts[len(linkPathPts)-1]
	}

	// Parse the target path
	targetPts, targetIsAbsolute := unixfs.SplitPath(oldPath)

	// Create the symbolic link
	err = parentFsh.Symlink(f.ctx, true, filename, targetPts, targetIsAbsolute, time.Now())
	if err != nil {
		return UnixfsErrorToWazeroErrno(err)
	}

	return 0
}

// Readlink reads the contents of a symbolic link.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - EINVAL: `path` is invalid.
//
// # Notes
//
//   - This is like syscall.Readlink, except the path is relative to this
//     filesystem.
//   - This is like `readlink` in POSIX. See
//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/readlink.html
func (f *FS) Readlink(path string) (string, wazero_exp_sys.Errno) {
	pathPts, errno := f.resolvePath(path)
	if errno != 0 {
		return "", errno
	}

	// Look up the symlink handle
	fileFsh, _, err := f.fsh.LookupPathPts(f.ctx, pathPts)
	if err != nil {
		if fileFsh != nil {
			fileFsh.Release()
		}
		return "", UnixfsErrorToWazeroErrno(err)
	}
	defer fileFsh.Release()

	// Read the symbolic link contents
	targetPts, targetIsAbsolute, err := fileFsh.Readlink(f.ctx, "")
	if err != nil {
		return "", UnixfsErrorToWazeroErrno(err)
	}

	// Convert path components back to string
	targetPath := unixfs.JoinPath(targetPts, targetIsAbsolute)
	return targetPath, 0
}

// Utimens set file access and modification times on a path relative to
// this file system, at nanosecond precision.
//
// # Parameters
//
// If the path is a symbolic link, the target of expanding that link is
// updated.
//
// The `atim` and `mtim` parameters refer to access and modification time
// stamps as defined in sys.Stat_t. To retain one or the other, substitute
// it with the pseudo-timestamp UTIME_OMIT.
//
// # Errors
//
// A zero Errno is success. The below are expected otherwise:
//   - ENOSYS: the implementation does not support this function.
//   - EINVAL: `path` is invalid.
//   - EEXIST: `path` exists and is a directory.
//   - ENOTDIR: `path` exists and is a file.
//
// # Notes
//
//   - This is like syscall.UtimesNano and `utimensat` with `AT_FDCWD` in
//     POSIX. See https://pubs.opengroup.org/onlinepubs/9699919799/functions/futimens.html
//   - Access times are ignored as the underlying UnixFS filesystem does not
//     support separate access and modification times.
func (f *FS) Utimens(path string, atim, mtim int64) wazero_exp_sys.Errno {
	// Only handle modification time updates, ignore access time
	if mtim == wazero_exp_sys.UTIME_OMIT {
		return 0 // Nothing to do
	}

	pathPts, errno := f.resolvePath(path)
	if errno != 0 {
		return errno
	}

	// Look up the file/directory handle, following symlinks
	fileFsh, _, err := f.fsh.LookupPathPts(f.ctx, pathPts)
	if err != nil {
		if fileFsh != nil {
			fileFsh.Release()
		}
		return UnixfsErrorToWazeroErrno(err)
	}
	defer fileFsh.Release()

	// Convert nanoseconds to time.Time
	mtime := time.Unix(mtim/1e9, mtim%1e9)

	// Set the modification timestamp
	err = fileFsh.SetModTimestamp(f.ctx, mtime)
	if err != nil {
		return UnixfsErrorToWazeroErrno(err)
	}

	return 0
}

// _ is a type assertion
var _ wazero_sysfs.FS = ((*FS)(nil))
