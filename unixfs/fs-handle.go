package unixfs

import (
	"context"
	"io/fs"
	"path"
	"sync/atomic"
	"time"

	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/pkg/errors"
)

// FSHandle is an open handle to a location in a FSTree.
// The handle may be released if the location ceases to exist.
type FSHandle struct {
	// isReleased indicates if this is released.
	isReleased atomic.Bool
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
	return h.isReleased.Load()
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
func (h *FSHandle) SetModTimestamp(ctx context.Context, mtime time.Time) error {
	return h.i.accessInode(ctx, func(ops FSCursorOps) error {
		return ops.SetModTimestamp(ctx, mtime)
	})
}

// ReadAt reads from a location in a File node.
func (h *FSHandle) ReadAt(ctx context.Context, offset int64, data []byte) (int64, error) {
	var read int64
	err := h.i.accessInode(ctx, func(ops FSCursorOps) error {
		if !ops.GetIsFile() {
			return unixfs_errors.ErrNotFile
		}

		var err error
		read, err = ops.ReadAt(ctx, offset, data)
		return err
	})
	return read, err
}

// Write writes to an offset in a file node synchronously.
// The change will be fully written to the file before returning.
// If this isn't a file node, returns ErrNotFile.
func (h *FSHandle) WriteAt(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	return h.i.accessInode(ctx, func(ops FSCursorOps) error {
		if !ops.GetIsFile() {
			return unixfs_errors.ErrNotFile
		}

		return ops.WriteAt(ctx, offset, data, ts)
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
func (h *FSHandle) ReaddirAll(ctx context.Context, skip uint64, cb func(ent FSCursorDirent) error) error {
	return h.i.accessInode(ctx, func(ops FSCursorOps) error {
		return ops.ReaddirAll(ctx, skip, cb)
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
//
// Use empty string "" or / for the root.
// Returns ErrNotExist if the entry was not found.
// Returns ErrReleased if the handle has been released.
func (h *FSHandle) LookupPath(ctx context.Context, filePath string) (*FSHandle, error) {
	filePath = path.Clean(filePath)
	if filePath == "/" || filePath == "." {
		filePath = ""
	}
	if filePath != "" && !fs.ValidPath(filePath) {
		return nil, &fs.PathError{
			Op:   "lookup",
			Path: filePath,
			Err:  fs.ErrInvalid,
		}
	}

	pathParts := SplitPath(filePath)
	return h.LookupPathPts(ctx, pathParts)
}

// LookupPathPts looks up a path with the path pre-split into parts.
func (h *FSHandle) LookupPathPts(ctx context.Context, pathParts []string) (*FSHandle, error) {
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

// Copy recursively copies a location to a destination, overwriting destination.
//
// The source and destination must be from the same inode tree.
func (h *FSHandle) Copy(ctx context.Context, dest *FSHandle, destName string, ts time.Time) error {
	if h == nil || dest == nil || dest.CheckReleased() || h.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if h.i.f != dest.i.f {
		return unixfs_errors.ErrInodeUnresolvable
	}
	if h == dest {
		// copy to self? no-op
		return nil
	}

	// access source inode
	return h.i.accessInode(ctx, func(srcOps FSCursorOps) error {
		// access destination inode
		return dest.i.accessInode(ctx, func(destOps FSCursorOps) error {
			// Attempt to perform optimized copy from src -> dest.
			done, err := srcOps.CopyTo(ctx, destOps, destName, ts)
			if err != nil || done {
				return err
			}

			// Attempt to perform optimized copy from dest <- src.
			done, err = destOps.CopyFrom(ctx, destName, srcOps, ts)
			if err != nil || done {
				return err
			}

			// No optimized path exists, do it the slow way.
			// TODO: recursive copy
			if le := h.i.f.le; le != nil {
				le.Warnf("TODO: cross-fs copy between locations: %#v -> %#v", srcOps, destOps)
			}
			return errors.Errorf("unable to copy between these locations")
		})
	})
}

// Rename recursively moves a source path to a destination, overwriting destination.
//
// The source and destination must be from the same inode tree.
func (h *FSHandle) Rename(ctx context.Context, dest *FSHandle, destName string, ts time.Time) error {
	if h == nil || dest == nil || dest.CheckReleased() || h.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if h.i.f != dest.i.f {
		return unixfs_errors.ErrInodeUnresolvable
	}
	if h == dest {
		// rename to self? no-op
		return nil
	}

	// access source inode
	err := h.i.accessInode(ctx, func(srcOps FSCursorOps) error {
		// access destination parent inode
		return dest.i.accessInode(ctx, func(destOps FSCursorOps) error {
			if srcOps.CheckReleased() || destOps.CheckReleased() {
				return unixfs_errors.ErrReleased
			}

			// Attempt to perform optimized move from src -> dest.
			done, err := srcOps.MoveTo(ctx, destOps, destName, ts)
			if err != nil || done {
				return err
			}

			// Attempt to perform optimized move from dest <- src.
			done, err = destOps.MoveFrom(ctx, destName, srcOps, ts)
			if err != nil || done {
				return err
			}

			// No optimized path exists, do it the slow way.
			// TODO: recursive move
			if le := h.i.f.le; le != nil {
				le.Warnf("TODO: cross-fs rename between locations: %#v -> %#v", srcOps, destOps)
			}
			return errors.Errorf("unable to rename between these locations")
		})
	})
	if err != nil {
		// note: might be good to expire the fsOps here
		return err
	}

	// successful rename: the source location has likely already been released,
	// and the destination location should have had a change callback called.

	// after the move: we need the old FSHandle and inode from source location
	// to remain valid, but point to the destination location. copy all
	// references and merge all children nodes to the destination.

	if err := h.i.f.waitSema.Acquire(ctx, 1); err != nil {
		return err
	}
	defer h.i.f.waitSema.Release(1)

	// remove the source inode from the parent children list
	srcLoc := h.i
	if parent := srcLoc.parent; parent != nil {
		oldChild, oldChildIdx := parent.findChildInode(h.i.name, false)
		if oldChild != nil {
			parent.removeChildInodeAtIdx(oldChildIdx)
		}
		if oldChild != srcLoc {
			oldChild.releaseWithChildrenLocked(unixfs_errors.ErrReleased)
		}
	}

	// destination parent was released; nothing further we can do from here.
	if dest.i.checkReleased() {
		h.i.releaseLocked(unixfs_errors.ErrReleased)
		return unixfs_errors.ErrReleased
	}

	// lookup or create the destination location
	destLoc, destLocIdx := dest.i.findChildInode(destName, true)
	if destLoc == nil {
		// child inode not found, insert at insertidx.
		destLoc = newFsInode(dest.i.f, dest.i, destName)
		dest.i.children = fsInodeSliceInsert(dest.i.children, destLocIdx, destLoc)
	}

	// merge srcLoc -> destLoc: moving refs and children
	destLoc.mergeWithNodeLocked(srcLoc, unixfs_errors.ErrReleased)

	// ensure h.i was updated, in case it was not already
	h.i = destLoc

	// done
	return nil
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
	if h.isReleased.Swap(true) {
		return
	}
	inode := h.i
	if err := inode.f.waitSema.Acquire(context.Background(), 1); err == nil {
		defer inode.f.waitSema.Release(1)
	}
	inode.removeRefLocked(h)
}
