package unixfs

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/helper/chroot"
	"github.com/go-git/go-billy/v5/util"
)

// BillyFS implements the Billy filesystem interface with a FSHandle.
type BillyFS struct {
	// ctx is the context
	ctx context.Context
	// h is the filesystem handle
	h *FSHandle
	// t is a constant timestamp
	t time.Time
}

// NewBillyFS constructs a new BillyFS from a FSHandle.
func NewBillyFS(ctx context.Context, h *FSHandle) *BillyFS {
	return &BillyFS{ctx: ctx, h: h}
}

// SetOpTimestamp sets the FS to use a single constant timestamp.
func (b *BillyFS) SetOpTimestamp(t time.Time) {
	b.t = t
}

// NewBillyFilesystem constructs the BillyFS and wraps it with chroot.
//
// note: also polyfills any unavailable features on BillyFS.
func NewBillyFilesystem(ctx context.Context, h *FSHandle, root string) billy.Filesystem {
	return chroot.New(NewBillyFS(ctx, h), root)
}

// timestamp returns the current timestamp.
func (f *BillyFS) timestamp() time.Time {
	if f.t.IsZero() {
		return time.Now()
	}
	return f.t
}

// Create creates the named file with mode 0666 (before umask), truncating
// it if it already exists. If successful, methods on the returned File can
// be used for I/O; the associated file descriptor has mode O_RDWR.
func (f *BillyFS) Create(filename string) (billy.File, error) {
	ts := f.timestamp()
	fileHandle, err := f.h.Lookup(f.ctx, filename)
	if err == unixfs_errors.ErrNotExist {
		err = f.h.Mknod(f.ctx, false, []string{filename}, NewFSCursorNodeType_File(), 0666, ts)
		fileHandle = nil
	} else if err == nil {
		err = fileHandle.Truncate(f.ctx, 0, ts)
	}
	if err == nil && fileHandle == nil {
		fileHandle, err = f.h.Lookup(f.ctx, filename)
	}
	if err != nil {
		if fileHandle != nil {
			fileHandle.Release()
		}
		return nil, err
	}
	return NewBillyFSFile(f.ctx, filename, fileHandle, os.O_CREATE|os.O_RDWR, f.t), nil
}

// Open opens the named file for reading. If successful, methods on the
// returned file can be used for reading; the associated file descriptor has
// mode O_RDONLY.
func (f *BillyFS) Open(filename string) (billy.File, error) {
	fileHandle, err := f.h.Lookup(f.ctx, filename)
	if err != nil {
		return nil, err
	}
	return NewBillyFSFile(f.ctx, filename, fileHandle, os.O_RDONLY, f.t), nil
}

// OpenFile is the generalized open call; most users will use Open or Create
// instead. It opens the named file with specified flag (O_RDONLY etc.) and
// perm, (0666 etc.) if applicable. If successful, methods on the returned
// File can be used for I/O.
func (f *BillyFS) OpenFile(filename string, flag int, perm os.FileMode) (billy.File, error) {
	fileHandle, err := f.h.Lookup(f.ctx, filename)
	isExcl := isExclusive(flag)
	if isExcl {
		if err == nil {
			fileHandle.Release()
			return nil, unixfs_errors.ErrExist
		}
		if err != unixfs_errors.ErrNotExist {
			return nil, err
		}
	}
	// TODO: resolve symlink
	/*
		if err == nil && fileHandle != nil && fileHandle.IsSymlink() {
			f.h.ResolveLink...
		}
	*/
	// create the file if necessary
	if err == unixfs_errors.ErrNotExist {
		if !isCreate(flag) {
			return nil, err
		}

		// create the file
		err = f.h.Mknod(
			f.ctx,
			isExcl,
			[]string{filename},
			NewFSCursorNodeType_File(),
			perm&fs.ModePerm,
			f.timestamp(),
		)
		if err != nil {
			return nil, err
		}

		// re-open the file again
		fileHandle, err = f.h.Lookup(f.ctx, filename)
		if err == unixfs_errors.ErrNotExist {
			// bug or race condition
			return nil, errors.New("file did not exist after being created")
		}
	}
	if err != nil {
		if fileHandle != nil {
			fileHandle.Release()
		}
		return nil, err
	}
	return NewBillyFSFile(f.ctx, filename, fileHandle, flag, f.t), nil
}

// Stat returns a FileInfo describing the named file.
func (f *BillyFS) Stat(filename string) (os.FileInfo, error) {
	fileHandle, err := f.h.Lookup(f.ctx, filename)
	if err != nil {
		return nil, err
	}
	defer fileHandle.Release()

	return fileHandle.GetFileInfo(f.ctx)
}

// Rename renames (moves) oldpath to newpath. If newpath already exists and
// is not a directory, Rename replaces it. OS-specific restrictions may
// apply when oldpath and newpath are in different directories.
func (f *BillyFS) Rename(oldpath, newpath string) error {
	return errors.New("TODO billyfs")
}

// Remove removes the named file or directory.
func (f *BillyFS) Remove(filename string) error {
	ts := f.timestamp()
	return f.h.Remove(f.ctx, []string{filename}, ts)
}

// Join joins any number of path elements into a single path, adding a
// Separator if necessary. Join calls filepath.Clean on the result; in
// particular, all empty strings are ignored. On Windows, the result is a
// UNC path if and only if the first path element is a UNC path.
func (f *BillyFS) Join(elem ...string) string {
	return filepath.Clean(filepath.Join(elem...))
}

// TempFile creates a new temporary file in the directory dir with a name
// beginning with prefix, opens the file for reading and writing, and
// returns the resulting *os.File. If dir is the empty string, TempFile
// uses the default directory for temporary files (see os.TempDir).
// Multiple programs calling TempFile simultaneously will not choose the
// same file. The caller can use f.Name() to find the pathname of the file.
// It is the caller's responsibility to remove the file when no longer
// needed.
func (f *BillyFS) TempFile(dir, prefix string) (billy.File, error) {
	// TODO: consider using OS temporary dir for this, instead of block graph.
	return util.TempFile(f, dir, prefix)
}

// ReadDir reads the directory named by dirname and returns a list of
// directory entries sorted by filename.
func (f *BillyFS) ReadDir(path string) ([]os.FileInfo, error) {
	if path == "" {
		return ReaddirAllToFileInfo(f.ctx, f.h)
	}

	ch, err := f.h.Lookup(f.ctx, path)
	if err != nil {
		return nil, err
	}
	defer ch.Release()

	return ReaddirAllToFileInfo(f.ctx, ch)
}

// MkdirAll creates a directory named path, along with any necessary
// parents, and returns nil, or else returns an error. The permission bits
// perm are used for all directories that MkdirAll creates. If path is/
// already a directory, MkdirAll does nothing and returns nil.
func (f *BillyFS) MkdirAll(filename string, perm os.FileMode) error {
	// check if exists
	fh, err := f.h.Lookup(f.ctx, filename)
	if err == unixfs_errors.ErrNotExist {
		fh = nil
	} else if err != nil {
		return err
	}
	if fh != nil {
		nt, err := fh.GetNodeType(f.ctx)
		fh.Release()
		if err != nil {
			return err
		}
		if nt.GetIsDirectory() {
			return nil
		} else {
			return unixfs_errors.ErrExist
		}
	}

	ts := f.timestamp()
	return f.h.Mknod(f.ctx, true, []string{filename}, NewFSCursorNodeType_Dir(), perm&fs.ModePerm, ts)
}

/* TODO: symlink support

// Lstat returns a FileInfo describing the named file. If the file is a symbolic
// link, the returned FileInfo describes the symbolic link. Lstat makes no
// attempt to follow the link.
func (f *BillyFS) Lstat(filename string) (os.FileInfo, error) {
	// TODO: symbolic links not supported.
	return f.Stat(filename)
}

// Symlink creates a symbolic-link from link to target. target may be an
// absolute or relative path, and need not refer to an existing node.
// Parent directories of link are created as necessary.
func (f *BillyFS) Symlink(target, link string) error {
	return errors.New("TODO: unixfs billy-fs: create symlink not supported")
}

// Readlink returns the target path of link.
func (f *BillyFS) Readlink(link string) (string, error) {
	fi, err := f.Lstat(link)
	if err != nil {
		return "", err
	}
	if !isSymlink(fi.Mode()) {
		return "", &os.PathError{
			Op:   "readlink",
			Path: link,
			Err:  unixfs_errors.ErrNotSymlink,
		}
	}
	return "", errors.New("TODO: unixfs billy-fs: symlink not supported")
}
*/

// Chmod changes the mode of the named file to mode. If the file is a
// symbolic link, it changes the mode of the link's target.
func (f *BillyFS) Chmod(name string, mode os.FileMode) error {
	info, err := f.h.GetFileInfo(f.ctx)
	if err != nil {
		return err
	}

	oldType := info.Mode() & fs.ModeType
	setType := mode & fs.ModeType
	if oldType != setType {
		return errors.New("TODO chmod: change node type")
	}

	oldPerms := info.Mode() & fs.ModePerm
	setPerms := mode & fs.ModePerm
	if oldPerms != setPerms {
		err = f.h.SetPermissions(f.ctx, setPerms, f.timestamp())
		if err != nil {
			return err
		}
	}
	return nil
}

// Lchown changes the numeric uid and gid of the named file. If the file is
// a symbolic link, it changes the uid and gid of the link itself.
func (f *BillyFS) Lchown(name string, uid, gid int) error {
	// TODO: chown
	return billy.ErrNotSupported
}

// Chown changes the numeric uid and gid of the named file. If the file is a
// symbolic link, it changes the uid and gid of the link's target.
func (f *BillyFS) Chown(name string, uid, gid int) error {
	// TODO: chown
	return billy.ErrNotSupported
}

// Chtimes changes the access and modification times of the named file,
// similar to the Unix utime() or utimes() functions.
//
// The underlying filesystem may truncate or round the values to a less
// precise time unit.
func (f *BillyFS) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return f.h.SetModTimestamp(f.ctx, mtime)
}

// _ is a type assertion
var (
	_ billy.Basic    = ((*BillyFS)(nil))
	_ billy.TempFile = ((*BillyFS)(nil))
	_ billy.Dir      = ((*BillyFS)(nil))
	_ billy.Change   = ((*BillyFS)(nil))
	// _ billy.Symlink  = ((*BillyFS)(nil))

	// note: use chroot helper
	// Chroot
	// _ billy.Filesystem = ((*BillyFS)(nil))
)
