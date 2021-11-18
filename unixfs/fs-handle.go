package unixfs

import (
	"context"
	"io/fs"
	"sync/atomic"
	"time"

	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// FSHandle is an open handle to a location in a FSTree.
// The handle may be released if the location ceases to exist.
type FSHandle struct {
	// isReleased is a uint32 atomic int
	isReleased uint32
	// i is the underlying inode
	i *fsInode
}

// newFSHandle constructs a new handle attached to fs.
func newFSHandle(i *fsInode) *FSHandle {
	return &FSHandle{i: i}
}

// TODO: AddChangeCb

// GetName returns the name of the inode.
func (h *FSHandle) GetName() string {
	return h.i.name
}

// CheckReleased checks if released without locking anything.
func (h *FSHandle) CheckReleased() bool {
	if atomic.LoadUint32(&h.isReleased) == 1 {
		return true
	}
	// note: if inode is released, it must have had an error attached.
	// we will check this error later.
	return false
}

// AccessOps accesses the inode operations.
func (h *FSHandle) AccessOps(ctx context.Context, cb func(ops FSCursorOps) error) error {
	return h.i.accessInode(ctx, cb)
}

// GetFileInfo constructs a file info object and creation time for the inode at handle.
func (h *FSHandle) GetFileInfo(ctx context.Context) (fs.FileInfo, error) {
	var fileInfo fs.FileInfo
	err := h.i.accessInode(ctx, func(ops FSCursorOps) error {
		permissions, err := ops.GetPermissions(ctx)
		if err != nil {
			return err
		}
		mode := NodeTypeToMode(ops, permissions)
		size, err := ops.GetSize(ctx)
		if err != nil {
			return err
		}
		modTime, err := ops.GetModTimestamp(ctx)
		if err != nil {
			return err
		}
		fileInfo = NewFileInfo(ops.GetName(), int64(size), mode, modTime)
		return nil
	})
	return fileInfo, err
}

// GetNodeType returns the FSCursor node type.
func (h *FSHandle) GetNodeType(ctx context.Context) (FSCursorNodeType, error) {
	var nodeType FSCursorNodeType
	err := h.i.accessInode(ctx, func(ops FSCursorOps) error {
		nodeType = ops
		return nil
	})
	return nodeType, err
}

// GetSize returns the size of the inode (in bytes).
// Usually applicable only if this is a FILE.
func (h *FSHandle) GetSize(ctx context.Context) (uint64, error) {
	var size uint64
	err := h.i.accessInode(ctx, func(ops FSCursorOps) error {
		var err error
		size, err = ops.GetSize(ctx)
		return err
	})
	return size, err
}

// GetOptimalWriteSize returns the optimal write size for the node.
// Usually applicable only if this is a FILE.
func (h *FSHandle) GetOptimalWriteSize(ctx context.Context) (int64, error) {
	var size int64
	err := h.i.accessInode(ctx, func(ops FSCursorOps) error {
		var err error
		size, err = ops.GetOptimalWriteSize(ctx)
		return err
	})
	return size, err
}

// GetModTimestamp returns the creation time and modification time.
func (h *FSHandle) GetModTimestamp(ctx context.Context) (mtime time.Time, err error) {
	err = h.i.accessInode(ctx, func(ops FSCursorOps) error {
		var err error
		mtime, err = ops.GetModTimestamp(ctx)
		return err
	})
	return
}

// GetPermissions returns the permissions bits of the file mode.
// The file mode portion of the value is ignored.
func (h *FSHandle) GetPermissions(ctx context.Context) (fm fs.FileMode, err error) {
	err = h.i.accessInode(ctx, func(ops FSCursorOps) error {
		var berr error
		fm, berr = ops.GetPermissions(ctx)
		return berr
	})
	return
}

// SetPermissions updates the permissions bits of the file mode.
// The file mode portion of the value is ignored.
func (h *FSHandle) SetPermissions(ctx context.Context, permissions fs.FileMode, t time.Time) error {
	return h.i.accessInode(ctx, func(ops FSCursorOps) error {
		return ops.SetPermissions(ctx, permissions, t)
	})
}

// SetModTimestamp updates the modification timestamp of the node.
func (h *FSHandle) SetModTimestamp(ctx context.Context, t time.Time) error {
	return h.i.accessInode(ctx, func(ops FSCursorOps) error {
		return ops.SetModTimestamp(ctx, t)
	})
}

// Read reads from a location in a File node.
func (h *FSHandle) Read(ctx context.Context, offset int64, data []byte) (int64, error) {
	var read int64
	err := h.i.accessInode(ctx, func(ops FSCursorOps) error {
		if !ops.GetIsFile() {
			return unixfs_errors.ErrNotFile
		}

		var err error
		read, err = ops.Read(ctx, offset, data)
		return err
	})
	return read, err
}

// Write writes to an offset in a file node synchronously.
// The change will be fully written to the file before returning.
// If this isn't a file node, returns ErrNotFile.
func (h *FSHandle) Write(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	return h.i.accessInode(ctx, func(ops FSCursorOps) error {
		if !ops.GetIsFile() {
			return unixfs_errors.ErrNotFile
		}

		return ops.Write(ctx, offset, data, ts)
	})
}

// Truncate shrinks or extends a file to the specified size.
// The extended part will be a sparse range (hole) reading as zeros.
func (h *FSHandle) Truncate(ctx context.Context, nsize uint64, ts time.Time) error {
	return h.i.accessInode(ctx, func(ops FSCursorOps) error {
		if !ops.GetIsFile() {
			return unixfs_errors.ErrNotFile
		}

		return ops.Truncate(ctx, nsize, ts)
	})
}

// ReaddirAll reads all directory entries.
func (h *FSHandle) ReaddirAll(ctx context.Context, cb func(ent FSCursorDirent) error) error {
	return h.i.accessInode(ctx, func(ops FSCursorOps) error {
		return ops.ReaddirAll(ctx, cb)
	})
}

// Lookup looks up a child entry in a directory.
// Returns ErrNotExist if the child entry was not found.
// Returns ErrReleased if the reference has been released.
// Creates a new FSCursor at the new location.
func (h *FSHandle) Lookup(ctx context.Context, name string) (*FSHandle, error) {
	return h.i.lookup(ctx, name)
}

// LookupPath recursively traverses the path, returning a handle pointing to the target.
func (h *FSHandle) LookupPath(ctx context.Context, filePath string) (*FSHandle, error) {
	pathParts := SplitPath(filePath)
	outHandle, err := h.Clone(ctx)
	if err != nil {
		return nil, err
	}
	for _, pathPart := range pathParts {
		if pathPart == "." {
			continue
		}

		nh, err := outHandle.Lookup(ctx, pathPart)
		outHandle.Release()
		if err != nil {
			return nil, err
		}
		outHandle = nh
	}
	return outHandle, nil
}

// Mknod creates child entries in a directory.
// inode must be a directory.
// passing 0 for permissions will set defaults
// any non-permissions bits in permissions will be ignored
// if checkExist, checks if name exists, returns ErrExist if so
func (h *FSHandle) Mknod(
	ctx context.Context,
	checkExist bool,
	names []string,
	nodeType FSCursorNodeType,
	permissions fs.FileMode,
	ts time.Time,
) error {
	return h.i.accessInode(ctx, func(ops FSCursorOps) error {
		return ops.Mknod(ctx, checkExist, names, nodeType, permissions, ts)
	})
}

// MkdirAll creates a directory named path, along with any necessary parents,
// and returns nil, or else returns an error. The permission bits perm are used
// for all directories that MkdirAll creates. If path is/ already a directory,
// MkdirAll does nothing and returns nil.
func (h *FSHandle) MkdirAll(ctx context.Context, filepath string, perm fs.FileMode, ts time.Time) error {
	if filepath == "" || filepath == "." {
		return nil
	}

	dirPath := SplitPath(filepath)
	dirHandle, err := h.Clone(ctx)
	if err != nil {
		return err
	}
	for _, pname := range dirPath {
		if pname == "." {
			continue
		}
		// check if exists
		dh, err := dirHandle.Lookup(ctx, pname)
		if err == unixfs_errors.ErrNotExist {
			// create dir
			err = dirHandle.Mknod(ctx, false, []string{pname}, NewFSCursorNodeType_Dir(), perm, ts)
			if err != nil {
				dirHandle.Release()
				return err
			}
			// lookup again
			dh, err = dirHandle.Lookup(ctx, pname)
		}
		dirHandle.Release()
		dirHandle = dh
		if err == nil {
			// check it is a dir
			var nt FSCursorNodeType
			nt, err = dirHandle.GetNodeType(ctx)
			if err == nil && !nt.GetIsDirectory() {
				err = unixfs_errors.ErrNotDirectory
			}
		}
		if err != nil {
			if dh != nil {
				dh.Release()
			}
			return err
		}
	}

	// done
	dirHandle.Release()
	return nil
}

// Symlink creates a symbolic link from a location to a path.
func (h *FSHandle) Symlink(ctx context.Context, checkExist bool, name string, target []string, ts time.Time) error {
	if len(name) == 0 || len(target) == 0 {
		return unixfs_errors.ErrEmptyPath
	}
	return h.i.accessInode(ctx, func(ops FSCursorOps) error {
		return ops.Symlink(ctx, checkExist, name, target, ts)
	})
}

// Readlink reads a symbolic link contents.
// If name is empty, reads the link at the FSHandle.
// Returns ErrNotSymlink if not a symbolic link.
func (h *FSHandle) Readlink(ctx context.Context, name string) ([]string, error) {
	handle := h
	if len(name) != 0 {
		var err error
		handle, err = h.Lookup(ctx, name)
		if err != nil {
			return nil, err
		}
		defer handle.Release()
	}

	var link []string
	err := handle.i.accessInode(ctx, func(ops FSCursorOps) error {
		var err error
		link, err = ops.Readlink(ctx, name)
		return err
	})
	return link, err
}

// Remove removes entries from a directory.
func (h *FSHandle) Remove(ctx context.Context, names []string, ts time.Time) error {
	if len(names) == 0 {
		return nil
	}
	return h.i.accessInode(ctx, func(ops FSCursorOps) error {
		return ops.Remove(ctx, names, ts)
	})
}

// Clone makes a copy of the FSHandle.
func (h *FSHandle) Clone(ctx context.Context) (*FSHandle, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := h.i.f.waitSema.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	defer h.i.f.waitSema.Release(1)

	rel := h.CheckReleased()
	if rel {
		return nil, unixfs_errors.ErrReleased
	}
	return h.i.addReference()
}

// Release releases the FSHandle.
func (h *FSHandle) Release() {
	if atomic.SwapUint32(&h.isReleased, 1) != 0 {
		return
	}
	inode := h.i
	if err := inode.f.waitSema.Acquire(context.Background(), 1); err == nil {
		defer inode.f.waitSema.Release(1)
	}
	inode.removeRefLocked(h)
}
