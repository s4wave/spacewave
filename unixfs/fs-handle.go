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
	// relCbs is a list of release callbacks
	// guarded by the inode waitSema
	relCbs []func()
}

// newFSHandle constructs a new handle attached to fs.
func newFSHandle(i *fsInode) *FSHandle {
	return &FSHandle{i: i}
}

// GetName returns the name of the inode.
func (h *FSHandle) GetName() string {
	return h.i.name
}

// CheckReleased checks if released without locking anything.
func (h *FSHandle) CheckReleased() bool {
	return h.isReleased.Load()
}

// AddReleaseCallback adds a callback that will be called when the FSHandle is released.
// May be called immediately (in the same call as AddReleaseCallback).
func (h *FSHandle) AddReleaseCallback(cb func()) {
	// fast path
	if h.CheckReleased() {
		cb()
		return
	}

	// slow path
	waitSema := h.i.f.waitSema
	relSema := waitSema.Acquire(context.Background(), 1) == nil
	if h.CheckReleased() {
		defer cb()
	} else {
		h.relCbs = append(h.relCbs, cb)
	}
	if relSema {
		h.i.f.waitSema.Release(1)
	}
}

// AccessOps accesses the FSCursor and FSCursorOps handles at the inode.
// It may take some time for the handles to be resolved.
// The handle and/or cursor may be released at any time and return unixfs_errors.ErrReleased.
//
// If ctx is canceled, returns context.Canceled.
// If cb returns unixfs_errors.ErrReleased, resolves the ops object & tries again.
// If cb returns any other value, returns that value.
// Note: do not call Release() on the FSCursorOps object.
func (h *FSHandle) AccessOps(ctx context.Context, cb func(cursor FSCursor, ops FSCursorOps) error) error {
	return h.i.accessInode(ctx, cb)
}

// GetOps resolves and returns the FSCursor and FSCursorOps once.
// Note: you may want to use AccessOps for the ErrReleased retry logic.
// Note: do not call Release() on the returned FSCursorOps object.
func (h *FSHandle) GetOps(ctx context.Context) (FSCursor, FSCursorOps, error) {
	var cursor FSCursor
	var val FSCursorOps
	err := h.AccessOps(ctx, func(fsCursor FSCursor, fsOps FSCursorOps) error {
		if fsOps.CheckReleased() || fsCursor.CheckReleased() {
			return unixfs_errors.ErrReleased
		}
		cursor, val = fsCursor, fsOps
		return nil
	})
	return cursor, val, err
}

// GetFileInfo constructs a file info object and creation time for the inode at handle.
func (h *FSHandle) GetFileInfo(ctx context.Context) (fs.FileInfo, error) {
	var fileInfo fs.FileInfo
	err := h.i.accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
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
	err := h.i.accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		nodeType = ops
		return nil
	})
	return nodeType, err
}

// GetSize returns the size of the inode (in bytes).
// Usually applicable only if this is a FILE.
func (h *FSHandle) GetSize(ctx context.Context) (uint64, error) {
	var size uint64
	err := h.i.accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
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
	err := h.i.accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		var err error
		size, err = ops.GetOptimalWriteSize(ctx)
		return err
	})
	return size, err
}

// GetModTimestamp returns the creation time and modification time.
func (h *FSHandle) GetModTimestamp(ctx context.Context) (mtime time.Time, err error) {
	err = h.i.accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		var err error
		mtime, err = ops.GetModTimestamp(ctx)
		return err
	})
	return
}

// GetPermissions returns the permissions bits of the file mode.
// The file mode portion of the value is ignored.
func (h *FSHandle) GetPermissions(ctx context.Context) (fm fs.FileMode, err error) {
	err = h.i.accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		var berr error
		fm, berr = ops.GetPermissions(ctx)
		return berr
	})
	return
}

// SetPermissions updates the permissions bits of the file mode.
// The file mode portion of the value is ignored.
func (h *FSHandle) SetPermissions(ctx context.Context, permissions fs.FileMode, t time.Time) error {
	return h.i.accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		return ops.SetPermissions(ctx, permissions, t)
	})
}

// SetModTimestamp updates the modification timestamp of the node.
func (h *FSHandle) SetModTimestamp(ctx context.Context, mtime time.Time) error {
	return h.i.accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		return ops.SetModTimestamp(ctx, mtime)
	})
}

// ReadAt reads from a location in a File node.
func (h *FSHandle) ReadAt(ctx context.Context, offset int64, data []byte) (int64, error) {
	var read int64
	err := h.i.accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
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
	return h.i.accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		if !ops.GetIsFile() {
			return unixfs_errors.ErrNotFile
		}

		return ops.WriteAt(ctx, offset, data, ts)
	})
}

// Truncate shrinks or extends a file to the specified size.
// The extended part will be a sparse range (hole) reading as zeros.
func (h *FSHandle) Truncate(ctx context.Context, nsize uint64, ts time.Time) error {
	return h.i.accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		if !ops.GetIsFile() {
			return unixfs_errors.ErrNotFile
		}

		return ops.Truncate(ctx, nsize, ts)
	})
}

// ReaddirAll reads all directory entries.
func (h *FSHandle) ReaddirAll(ctx context.Context, skip uint64, cb func(ent FSCursorDirent) error) error {
	return h.i.accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
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
// Returns the subset of filePath that was traversed.
//
// Use empty string "" or / for the root.
// Returns ErrNotExist if the entry was not found.
// Returns ErrReleased if the handle has been released.
//
// ErrNotExist is returned if not found and the FSHandle of the parent of the
// element, and the subset of pathParts that is the path to the returned node.
// Check if fsHandle is not nil and release it even if an error is returned.
func (h *FSHandle) LookupPath(ctx context.Context, filePath string) (*FSHandle, []string, error) {
	filePath = path.Clean(filePath)
	if filePath == "/" || filePath == "." {
		filePath = ""
	}
	if filePath != "" && filePath[0] == PathSeparator {
		filePath = filePath[1:]
	}
	if filePath != "" && !fs.ValidPath(filePath) {
		return nil, nil, &fs.PathError{
			Op:   "lookup",
			Path: filePath,
			Err:  fs.ErrInvalid,
		}
	}

	pathParts := SplitPath(filePath)
	return h.LookupPathPts(ctx, pathParts)
}

// LookupPathPts looks up a path with the path components split into parts.
// Returns the subset of pathParts that were traversed.
//
// ErrNotExist is returned if not found and the FSHandle of the parent of the
// element, and the subset of pathParts that is the path to the returned node.
// Check if fsHandle is not nil and release it even if an error is returned.
func (h *FSHandle) LookupPathPts(ctx context.Context, pathParts []string) (*FSHandle, []string, error) {
	outHandle, err := h.Clone(ctx)
	if err != nil {
		return nil, nil, err
	}
	for i, pathPart := range pathParts {
		if pathPart == "." {
			continue
		}

		nh, err := outHandle.Lookup(ctx, pathPart)
		if err != nil {
			return outHandle, pathParts[:i], err
		}
		outHandle.Release() // release parent handle
		outHandle = nh
	}
	return outHandle, pathParts, nil
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
	return h.i.accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
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
	return h.i.accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
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
	err := handle.i.accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
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
		// TODO: copy to self?
		return nil
	}

	// access source inode
	return h.i.accessInode(ctx, func(_ FSCursor, srcOps FSCursorOps) error {
		// access destination inode
		return dest.i.accessInode(ctx, func(_ FSCursor, destOps FSCursorOps) error {
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
	err := h.i.accessInode(ctx, func(_ FSCursor, srcOps FSCursorOps) error {
		// access destination parent inode
		return dest.i.accessInode(ctx, func(_ FSCursor, destOps FSCursorOps) error {
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
	return h.i.accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
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
		// already released
		return
	}
	waitSema := h.i.f.waitSema
	relSema := waitSema.Acquire(context.Background(), 1) == nil
	h.i.removeRefLocked(h)
	relCbs := h.relCbs
	h.relCbs = nil
	if relSema {
		waitSema.Release(1)
	}
	for _, cb := range relCbs {
		cb()
	}
}
