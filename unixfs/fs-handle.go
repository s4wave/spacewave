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
	return h.i.checkReleased()
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

// Write writes to an offset in a file node.
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

// Mknod creates child entries in a directory.
// inode must be a directory.
// passing 0 will set default permissions
// if checkExist, checks if name exists, returns ErrExist if so
func (h *FSHandle) Mknod(
	ctx context.Context,
	checkExist bool,
	names []string,
	nodeType FSCursorNodeType,
	permissions uint32,
	ts time.Time,
) error {
	return h.i.accessInode(ctx, func(ops FSCursorOps) error {
		return ops.Mknod(ctx, checkExist, names, nodeType, permissions, ts)
	})
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
