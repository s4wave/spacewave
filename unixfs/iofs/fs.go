package unixfs_iofs

import (
	"context"
	"io"
	"io/fs"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// IoFS is the set of interfaces FS implements.
type IoFS interface {
	fs.FS
	fs.ReadDirFS
	fs.ReadFileFS
	fs.StatFS
}

// FS implements the fs.FS interface with a FSHandle.
type FS struct {
	// ctx is the context to use for ops
	ctx context.Context
	// handle is the reference to the unixfs
	handle *unixfs.FSHandle
	// ignorePath ignores the passed path in Open
	ignorePath bool
}

// NewFS constructs a new fs.FS from a FSHandle.
//
// Returns nil if handle == nil.
// If ignorePath is set: all ops are applied to handle, ignoring the passed path.
func NewFS(ctx context.Context, handle *unixfs.FSHandle, opts ...FSOption) IoFS {
	if handle == nil {
		return nil
	}
	f := &FS{ctx: ctx, handle: handle}
	for _, opt := range opts {
		if opt != nil {
			opt(f)
		}
	}
	return f
}

// GetHandle returns the root FSHandle.
func (f *FS) GetHandle() *unixfs.FSHandle {
	return f.handle
}

// Open opens the named file.
//
// When Open returns an error, it usually will be of type *PathError with the Op
// field set to "open", the Path field set to name, and the Err field describing
// the problem.
//
// Open rejects attempts to open names that do not satisfy ValidPath(name),
// returning a *PathError with Err set to ErrInvalid or ErrNotExist.
func (f *FS) Open(name string) (fs.File, error) {
	if f.ignorePath {
		name = ""
	} else if err := f.checkFilePath(name); err != nil {
		return nil, err
	} else if name == "/" || name == "." {
		name = ""
	}

	// lookup the path to the file
	fsHandle, _, err := f.handle.LookupPath(f.ctx, name)
	if err != nil {
		if fsHandle != nil {
			fsHandle.Release()
		}
		if pathErr, pathErrOk := err.(*fs.PathError); pathErrOk {
			pathErr.Op = "open"
			return nil, pathErr
		} else if err == unixfs_errors.ErrNotExist {
			return nil, &fs.PathError{
				Op:   "open",
				Path: name,
				Err:  fs.ErrNotExist,
			}
		}
		return nil, err
	}

	return NewFSFile(f.ctx, fsHandle), nil
}

// Stat returns a FileInfo describing the file.
// If there is an error, it should be of type *PathError.
func (f *FS) Stat(name string) (fs.FileInfo, error) {
	if f.ignorePath {
		name = ""
	} else if err := f.checkFilePath(name); err != nil {
		return nil, err
	} else if name == "/" || name == "." {
		name = ""
	}

	h, _, err := f.handle.LookupPath(f.ctx, name)
	if err != nil {
		if h != nil {
			h.Release()
		}
		if err == unixfs_errors.ErrNotExist {
			return nil, &fs.PathError{
				Op:   "stat",
				Path: name,
				Err:  err,
			}
		}
		return nil, err
	}
	defer h.Release()

	return h.GetFileInfo(f.ctx)
}

// ReadDir reads the named directory
// and returns a list of directory entries sorted by filename.
func (f *FS) ReadDir(name string) ([]fs.DirEntry, error) {
	if f.ignorePath {
		name = ""
	} else if err := f.checkFilePath(name); err != nil {
		return nil, err
	} else if name == "/" || name == "." {
		name = ""
	}

	h, _, err := f.handle.LookupPath(f.ctx, name)
	if err != nil {
		if h != nil {
			h.Release()
		}
		return nil, err
	}
	defer h.Release()

	return unixfs.ReaddirAllToDirEntries(f.ctx, 0, 0, h)
}

// ReadFile reads the named file and returns its contents.
// A successful call returns a nil error, not io.EOF.
// (Because ReadFile reads the whole file, the expected EOF
// from the final Read is not treated as an error to be reported.)
//
// The caller is permitted to modify the returned byte slice.
// This method should return a copy of the underlying data.
func (f *FS) ReadFile(name string) ([]byte, error) {
	if f.ignorePath {
		name = ""
	} else if err := f.checkFilePath(name); err != nil {
		return nil, err
	} else if name == "/" || name == "." {
		name = ""
	}

	h, _, err := f.handle.LookupPath(f.ctx, name)
	if err != nil {
		if h != nil {
			h.Release()
		}
		if err == unixfs_errors.ErrNotExist {
			return nil, &fs.PathError{
				Op:   "read",
				Path: name,
				Err:  err,
			}
		}
		return nil, err
	}
	defer h.Release()

	// err = h.AccessOps(f.ctx,  func(ops unixfs.FSCursorOps) error {})
	size, err := h.GetSize(f.ctx)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, nil
	}
	// cap size at MaxReadFileSize
	if size > unixfs.MaxReadFileSize {
		return nil, unixfs.NewReadFileSizeTooLargeError(size)
	}
	data := make([]byte, size)
	fileHandle := NewFSFile(f.ctx, h)
	defer fileHandle.Close()
	_, err = io.ReadFull(fileHandle, data)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// checkFilePath checks the path using the fs.FS rules.
func (f *FS) checkFilePath(name string) error {
	// check path using fs.FS rules before path.Clean
	if !fs.ValidPath(name) {
		return &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	return nil
}

// _ is a type assertion
var _ IoFS = ((*FS)(nil))
