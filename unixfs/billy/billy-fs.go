package unixfs_billy

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/util"
)

// BillyFS implements the Billy filesystem interface with a FSHandle.
//
// NOTE: filename arguments support paths containing separators (/)
type BillyFS struct {
	// ctx is the context
	ctx context.Context
	// h is the filesystem handle
	h *unixfs.FSHandle
	// t is the write timestamp
	t atomic.Pointer[time.Time]
	// basePath is the path of any parent chroots
	basePath string
}

// NewBillyFS constructs a new BillyFS from a FSHandle.
//
// if ts is nil, uses time.Now() on each call
// basePath should contain the path to this FSHandle.
func NewBillyFS(ctx context.Context, h *unixfs.FSHandle, basePath string, ts time.Time) *BillyFS {
	if basePath == "" {
		basePath = "/"
	}
	basePath = path.Clean(basePath)
	bfs := &BillyFS{
		ctx:      ctx,
		h:        h,
		basePath: basePath,
	}
	if !ts.IsZero() {
		bfs.SetOpTimestamp(ts)
	}
	return bfs
}

// SetOpTimestamp sets the timestamp for FS write operations.
func (f *BillyFS) SetOpTimestamp(t time.Time) {
	f.t.Store(&t)
}

// GetOpTimestamp returns the current timestamp set to use for writes.
func (f *BillyFS) GetOpTimestamp() time.Time {
	ts := f.t.Load()
	if ts == nil {
		return time.Time{}
	}
	return *ts
}

// NewBillyFilesystem constructs the BillyFS.
//
// if ts is nil, uses time.Now() on each call
// basePath should contain the path to this FSHandle.
func NewBillyFilesystem(ctx context.Context, h *unixfs.FSHandle, basePath string, ts time.Time) billy.Filesystem {
	return NewBillyFS(ctx, h, basePath, ts)
}

// timestamp returns the timestamp to use for writes..
func (f *BillyFS) timestamp() time.Time {
	t := f.GetOpTimestamp()
	if t.IsZero() {
		return time.Now()
	}
	return t
}

// Root returns the root path of the filesystem.
func (f *BillyFS) Root() string {
	return f.basePath
}

// Create creates the named file with mode 0666 (before umask), truncating
// it if it already exists. If successful, methods on the returned File can
// be used for I/O; the associated file descriptor has mode O_RDWR.
func (f *BillyFS) Create(filepath string) (billy.File, error) {
	// note: this is the same behavior as os.Create
	return f.OpenFile(filepath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

// Open opens the named file for reading. If successful, methods on the
// returned file can be used for reading; the associated file descriptor has
// mode O_RDONLY.
func (f *BillyFS) Open(filepath string) (billy.File, error) {
	fileHandle, _, err := f.h.LookupPath(f.ctx, filepath)
	if err != nil {
		if fileHandle != nil {
			fileHandle.Release()
		}
		return nil, err
	}
	return NewBillyFSFile(f.ctx, fileHandle.GetName(), fileHandle, os.O_RDONLY, f.timestamp()), nil
}

// OpenFile is the generalized open call; most users will use Open or Create
// instead. It opens the named file with specified flag (O_RDONLY etc.) and
// perm, (0666 etc.) if applicable. If successful, methods on the returned
// File can be used for I/O.
func (f *BillyFS) OpenFile(filepath string, flag int, perm os.FileMode) (billy.File, error) {
	filepath = path.Clean(filepath)
	filedir, filename := path.Split(filepath)

	if flag&os.O_CREATE != 0 {
		// billyfs expects: create directories as needed
		if len(filedir) != 0 && filedir != "." {
			if err := f.MkdirAll(filedir, 0o755); err != nil {
				return nil, err
			}
		}
	}

	var h *unixfs.FSHandle
	if len(filedir) == 0 || filedir == "." {
		h = f.h
	} else {
		dirHandle, _, err := f.h.LookupPath(f.ctx, filedir)
		if err != nil {
			if dirHandle != nil {
				dirHandle.Release()
			}
			return nil, err
		}
		defer dirHandle.Release()
		h = dirHandle
	}

	fileHandle, err := h.Lookup(f.ctx, filename)
	isExcl := unixfs.FlagIsExclusive(flag)
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
		if !unixfs.FlagIsCreate(flag) {
			return nil, err
		}

		// create the file
		err = h.Mknod(
			f.ctx,
			isExcl,
			[]string{filename},
			unixfs.NewFSCursorNodeType_File(),
			perm&fs.ModePerm,
			f.timestamp(),
		)
		if err != nil {
			return nil, err
		}

		// re-open the file again
		fileHandle, err = h.Lookup(f.ctx, filename)
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
	return NewBillyFSFile(f.ctx, fileHandle.GetName(), fileHandle, flag, f.timestamp()), nil
}

// Stat returns a FileInfo describing the named file.
func (f *BillyFS) Stat(filepath string) (os.FileInfo, error) {
	return unixfs.StatWithPath(f.ctx, f.h, filepath)
}

// Rename renames (moves) oldpath to newpath. If newpath already exists and
// is not a directory, Rename replaces it. OS-specific restrictions may
// apply when oldpath and newpath are in different directories.
func (f *BillyFS) Rename(oldpath, newpath string) error {
	return unixfs.RenameWithPaths(f.ctx, f.h, oldpath, newpath, f.timestamp())
}

// Remove removes the named file or directory.
func (f *BillyFS) Remove(filepath string) error {
	return unixfs.RemoveAllWithPath(f.ctx, f.h, filepath, f.timestamp())
}

// Join joins any number of path elements into a single path, adding a
// Separator if necessary. Join calls filepath.Clean on the result; in
// particular, all empty strings are ignored. On Windows, the result is a
// UNC path if and only if the first path element is a UNC path.
func (f *BillyFS) Join(elem ...string) string {
	return path.Clean(path.Join(elem...))
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
func (f *BillyFS) ReadDir(mpath string) ([]os.FileInfo, error) {
	mpath = path.Clean(mpath)
	if mpath == "" || mpath == "." || mpath == "/" {
		return unixfs.ReaddirAllToFileInfo(f.ctx, 0, 0, f.h)
	}

	ch, _, err := f.h.LookupPath(f.ctx, mpath)
	if err != nil {
		if ch != nil {
			ch.Release()
		}
		return nil, err
	}
	defer ch.Release()

	return unixfs.ReaddirAllToFileInfo(f.ctx, 0, 0, ch)
}

// MkdirAll creates a directory named path, along with any necessary
// parents, and returns nil, or else returns an error. The permission bits
// perm are used for all directories that MkdirAll creates. If path is/
// already a directory, MkdirAll does nothing and returns nil.
func (f *BillyFS) MkdirAll(filepath string, perm os.FileMode) error {
	// lookup and/or create all path components
	ts := f.timestamp()
	return f.h.MkdirAllPath(f.ctx, filepath, perm, ts)
}

// Chmod changes the mode of the named file to mode. If the file is a
// symbolic link, it changes the mode of the link's target.
func (f *BillyFS) Chmod(filepath string, mode os.FileMode) error {
	return unixfs.ChmodWithPath(f.ctx, f.h, filepath, mode, f.timestamp())
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
func (f *BillyFS) Chtimes(filepath string, atime time.Time, mtime time.Time) error {
	if mtime.IsZero() {
		mtime = f.timestamp()
	}
	return unixfs.SetModTimestampWithPath(f.ctx, f.h, filepath, mtime)
}

// Lstat returns a FileInfo describing the named file. If the file is a
// symbolic link, the returned FileInfo describes the symbolic link. Lstat
// makes no attempt to follow the link.
func (f *BillyFS) Lstat(filepath string) (os.FileInfo, error) {
	// TODO TODO: this will traverse symbolic links: Lstat should not.
	return unixfs.StatWithPath(f.ctx, f.h, filepath)
}

// Symlink creates a symbolic-link from link to target. target may be an
// absolute or relative path, and need not refer to an existing node.
// Parent directories of link are created as necessary.
func (f *BillyFS) Symlink(target, link string) error {
	filepath := path.Clean(link)
	filedir, filename := path.Split(filepath)
	ch, _, err := f.h.LookupPath(f.ctx, filedir)
	if err != nil {
		if ch != nil {
			ch.Release()
		}
		return err
	}
	defer ch.Release()

	tgtComponents, tgtComponentsIsAbsolute := unixfs.SplitPath(target)
	return ch.Symlink(f.ctx, true, filename, tgtComponents, tgtComponentsIsAbsolute, f.timestamp())
}

// Readlink returns the target path of link.
func (f *BillyFS) Readlink(link string) (string, error) {
	ch, _, err := f.h.LookupPath(f.ctx, link)
	if err != nil {
		if ch != nil {
			ch.Release()
		}
		return "", err
	}
	defer ch.Release()
	nt, err := ch.GetNodeType(f.ctx)
	if err != nil {
		return "", err
	}
	if !nt.GetIsSymlink() {
		return "", &os.PathError{
			Op:   "readlink",
			Path: link,
			Err:  unixfs_errors.ErrNotSymlink,
		}
	}
	lnkd, lnkdAbsolute, err := ch.Readlink(f.ctx, "")
	if err != nil {
		return "", err
	}
	return unixfs.JoinPath(lnkd, lnkdAbsolute), nil
}

// Chroot returns a new filesystem from the same type where the new root is
// the given path. Files outside of the designated directory tree cannot be
// accessed.
func (f *BillyFS) Chroot(p string) (billy.Filesystem, error) {
	lh, _, err := f.h.LookupPath(f.ctx, p)
	if err != nil {
		if lh != nil {
			lh.Release()
		}
		return nil, err
	}
	nextBasePath := path.Join(f.basePath, p)
	return NewBillyFS(f.ctx, lh, nextBasePath, f.timestamp()), nil
}

// _ is a type assertion
var (
	_ billy.Basic      = ((*BillyFS)(nil))
	_ billy.TempFile   = ((*BillyFS)(nil))
	_ billy.Dir        = ((*BillyFS)(nil))
	_ billy.Change     = ((*BillyFS)(nil))
	_ billy.Symlink    = ((*BillyFS)(nil))
	_ billy.Chroot     = ((*BillyFS)(nil))
	_ billy.Filesystem = ((*BillyFS)(nil))
)
