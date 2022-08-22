package unixfs

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"sync/atomic"
	"syscall"
	"time"

	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/spf13/afero"
)

// AferoFSName is the constant name used for Hydra UnixFS Afero filesystems.
const AferoFSName = "hydra/unixfs"

// AferoFS implements the Afero filesystem interface with a FSHandle.
//
// NOTE: filename arguments support paths containing separators (/)
type AferoFS struct {
	// ctx is the context
	ctx context.Context
	// h is the filesystem handle
	h *FSHandle
	// t is a constant write timestamp
	t atomic.Pointer[time.Time]
	// basePath is the path of any parent chroots
	basePath string
}

// NewAferoFS constructs a new AferoFS from a FSHandle.
//
// if ts is nil, uses time.Now() on each call
// basePath should contain the path to this FSHandle.
func NewAferoFS(ctx context.Context, h *FSHandle, basePath string, ts time.Time) *AferoFS {
	if basePath == "" {
		basePath = "/"
	}
	basePath = path.Clean(basePath)
	afs := &AferoFS{ctx: ctx, h: h, basePath: basePath}
	if !ts.IsZero() {
		afs.SetOpTimestamp(ts)
	}
	return afs
}

// SetOpTimestamp sets the timestamp for FS write operations.
func (f *AferoFS) SetOpTimestamp(t time.Time) {
	f.t.Store(&t)
}

// GetOpTimestamp returns the current timestamp set to use for writes.
func (f *AferoFS) GetOpTimestamp() time.Time {
	ts := f.t.Load()
	if ts == nil {
		return time.Time{}
	}
	return *ts
}

// Name  of this FileSystem
func (f *AferoFS) Name() string {
	return AferoFSName
}

// Create creates a file in the filesystem, returning the file and an
// error, if any happens.
func (f *AferoFS) Create(name string) (afero.File, error) {
	// note: this is the same behavior as os.Create
	return f.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
}

// Mkdir creates a directory in the filesystem, return an error if any happens.
// Returns an error if the path already exists.
func (f *AferoFS) Mkdir(name string, perm os.FileMode) error {
	// lookup and/or create all path components
	ts := f.timestamp()
	name = path.Clean(name)

	createDir := path.Base(name)
	parentDir := path.Dir(name)

	// lookup parent directory
	fsh, err := f.h.LookupPath(f.ctx, parentDir)
	if err != nil {
		return err
	}
	defer fsh.Release()

	// mkdir operation
	return fsh.Mknod(
		f.ctx,
		true,
		[]string{createDir},
		NewFSCursorNodeType_Dir(),
		perm,
		ts,
	)
}

// MkdirAll creates a directory path and all parents that does not exist
// yet.
func (f *AferoFS) MkdirAll(mpath string, perm os.FileMode) error {
	mpath = path.Clean(mpath)

	fsh, err := f.h.Clone(f.ctx)
	if err != nil {
		return err
	}
	defer fsh.Release()

	return fsh.MkdirAll(f.ctx, mpath, perm, f.timestamp())
}

// Open opens a file, returning it or an error, if any happens.
func (f *AferoFS) Open(name string) (afero.File, error) {
	fileHandle, err := f.h.LookupPath(f.ctx, name)
	if err != nil {
		return nil, err
	}
	return NewAferoFSFile(f.ctx, fileHandle.GetName(), fileHandle, os.O_RDONLY, f.timestamp()), nil
}

// OpenFile opens a file using the given flags and the given mode.
func (f *AferoFS) OpenFile(filepath string, flag int, perm os.FileMode) (afero.File, error) {
	filepath = path.Clean(filepath)
	filedir, filename := path.Split(filepath)

	var h *FSHandle
	if filedir == "." {
		h = f.h
	} else {
		dirHandle, err := f.h.LookupPath(f.ctx, filedir)
		if err != nil {
			return nil, err
		}
		defer dirHandle.Release()
		h = dirHandle
	}

	fileHandle, err := h.Lookup(f.ctx, filename)
	isExcl := isExclusive(flag)
	if isExcl {
		if err == nil {
			fileHandle.Release()
			return nil, syscall.EEXIST
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
		err = h.Mknod(
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

		// TODO: requires a slight delay for the fscursors to update
		// TODO: This is a bug that currently is being fixed
		<-time.After(time.Millisecond * 10)

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
	return NewAferoFSFile(f.ctx, fileHandle.GetName(), fileHandle, flag, f.timestamp()), nil
}

// Remove removes a file identified by name, returning an error, if any happens.
func (f *AferoFS) Remove(filepath string) error {
	return f.RemoveAll(filepath)
}

// RemoveAll removes a directory path and any children it contains. It does not
// fail if the path does not exist (return nil).
func (f *AferoFS) RemoveAll(filepath string) error {
	return RemoveAllWithPath(f.ctx, f.h, filepath, f.timestamp())
}

// Rename renames a file.
func (f *AferoFS) Rename(oldpath, newpath string) error {
	return RenameWithPaths(f.ctx, f.h, oldpath, newpath, f.timestamp())
}

// Stat returns a FileInfo describing the named file, or any error.
func (f *AferoFS) Stat(filepath string) (fs.FileInfo, error) {
	return StatWithPath(f.ctx, f.h, filepath)
}

// Chmod changes the mode of the named file to mode.
func (f *AferoFS) Chmod(filepath string, mode os.FileMode) error {
	return ChmodWithPath(f.ctx, f.h, filepath, mode, f.timestamp())
}

// Chtimes changes the access and modification times of the named file
func (f *AferoFS) Chtimes(name string, atime time.Time, mtime time.Time) error {
	if mtime.IsZero() {
		mtime = f.timestamp()
	}
	return SetModTimestampWithPath(f.ctx, f.h, name, mtime)
}

// timestamp returns the timestamp to use for writes.
func (f *AferoFS) timestamp() time.Time {
	t := f.GetOpTimestamp()
	if t.IsZero() {
		return time.Now()
	}
	return t
}

// _ is a type assertion
var _ afero.Fs = ((*AferoFS)(nil))
