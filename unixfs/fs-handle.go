package unixfs

import (
	"context"
	"io/fs"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/aperturerobotics/util/cqueue"
	"github.com/aperturerobotics/util/refcount"
	"github.com/pkg/errors"
)

// FSHandle is an open handle to a location in a FSTree.
// The handle may be released if the location ceases to exist.
type FSHandle struct {
	// isReleased indicates if this is released.
	isReleased atomic.Bool
	// inode is the underlying inode
	// this field changes only if the inode was moved to a different cursor path.
	inode atomic.Pointer[fsInode]
	// relCbs is an atomic last-in-first-out set of callbacks
	relCbs cqueue.AtomicLIFO[func()]
}

// FSHandleBuilder satisfies the refcount resolver with a FSHandle.
type FSHandleBuilder = refcount.RefCountResolver[*FSHandle]

// NewFSHandle constructs a new FSHandle with a FSCursor.
// Note: the FSHandle will be released if the FSCursor is released.
func NewFSHandle(cursor FSCursor) (*FSHandle, error) {
	inode := newFsInode(nil, "", []FSCursor{cursor})
	return inode.addReferenceLocked(true)
}

// NewFSHandleWithPrefix constructs a new FSHandle with a FSCursor and follows a prefix.
//
// if mkdirPath is set, we will attempt to mkdir all elements of prefixPath.
func NewFSHandleWithPrefix(
	ctx context.Context,
	cursor FSCursor,
	prefixPath []string,
	mkdirPath bool,
	ts time.Time,
) (*FSHandle, error) {
	rootHandle, err := NewFSHandle(cursor)
	if err != nil {
		return nil, err
	}

	if len(prefixPath) == 0 {
		return rootHandle, nil
	}

	// if we should mkdirPath, ensure the prefix exists first.
	if mkdirPath {
		err = rootHandle.MkdirAll(
			ctx,
			prefixPath,
			DefaultPermissions(NewFSCursorNodeType_Dir()),
			ts,
		)
		if err != nil {
			rootHandle.Release()
			return nil, err
		}
	}

	// follow the new prefix path
	handle, _, err := rootHandle.LookupPathPts(ctx, prefixPath)
	// release the old root
	rootHandle.Release()
	// return the new handle
	return handle, err
}

// GetName returns the name of the inode.
func (h *FSHandle) GetName() string {
	return h.i().name
}

// CheckReleased checks if released without locking anything.
func (h *FSHandle) CheckReleased() bool {
	return h.isReleased.Load()
}

// AddReleaseCallback adds a callback that will be called when the FSHandle is released.
// May be called immediately (in the same call as AddReleaseCallback).
func (h *FSHandle) AddReleaseCallback(rcb func()) {
	if rcb == nil {
		return
	}

	var once sync.Once
	cb := func() {
		once.Do(rcb)
	}
	h.relCbs.Push(rcb)

	// fast path
	if h.CheckReleased() {
		cb()
		return
	}

	// slow path
	for {
		inode := h.i()
		if inode.checkReleased() {
			cb()
			return
		}
		inode.relCbs.Push(rcb)
		// if the inode was released or changed on h, continue & retry
		if inode.checkReleased() || h.i() != inode {
			continue
		}
		break
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
	return h.i().accessInode(ctx, cb)
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
	err := h.i().accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
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
	err := h.i().accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		nodeType = ops
		return nil
	})
	return nodeType, err
}

// GetSize returns the size of the inode (in bytes).
// Usually applicable only if this is a FILE.
func (h *FSHandle) GetSize(ctx context.Context) (uint64, error) {
	var size uint64
	err := h.i().accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
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
	err := h.i().accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		var err error
		size, err = ops.GetOptimalWriteSize(ctx)
		return err
	})
	return size, err
}

// GetModTimestamp returns the creation time and modification time.
func (h *FSHandle) GetModTimestamp(ctx context.Context) (mtime time.Time, err error) {
	err = h.i().accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		var err error
		mtime, err = ops.GetModTimestamp(ctx)
		return err
	})
	return
}

// GetPermissions returns the permissions bits of the file mode.
// The file mode portion of the value is ignored.
func (h *FSHandle) GetPermissions(ctx context.Context) (fm fs.FileMode, err error) {
	err = h.i().accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		var berr error
		fm, berr = ops.GetPermissions(ctx)
		return berr
	})
	return
}

// SetPermissions updates the permissions bits of the file mode.
// The file mode portion of the value is ignored.
func (h *FSHandle) SetPermissions(ctx context.Context, permissions fs.FileMode, t time.Time) error {
	return h.i().accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		return ops.SetPermissions(ctx, permissions, t)
	})
}

// SetModTimestamp updates the modification timestamp of the node.
func (h *FSHandle) SetModTimestamp(ctx context.Context, mtime time.Time) error {
	return h.i().accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		return ops.SetModTimestamp(ctx, mtime)
	})
}

// ReadAt reads from a location in a File node.
func (h *FSHandle) ReadAt(ctx context.Context, offset int64, data []byte) (int64, error) {
	var read int64
	err := h.i().accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
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
	return h.i().accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		if !ops.GetIsFile() {
			return unixfs_errors.ErrNotFile
		}

		return ops.WriteAt(ctx, offset, data, ts)
	})
}

// Truncate shrinks or extends a file to the specified size.
// The extended part will be a sparse range (hole) reading as zeros.
func (h *FSHandle) Truncate(ctx context.Context, nsize uint64, ts time.Time) error {
	return h.i().accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		if !ops.GetIsFile() {
			return unixfs_errors.ErrNotFile
		}

		return ops.Truncate(ctx, nsize, ts)
	})
}

// ReaddirAll reads all directory entries.
func (h *FSHandle) ReaddirAll(ctx context.Context, skip uint64, cb func(ent FSCursorDirent) error) error {
	return h.i().accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		return ops.ReaddirAll(ctx, skip, cb)
	})
}

// Lookup looks up a child entry in a directory.
// Returns ErrNotExist if the child entry was not found.
// Returns ErrReleased if the reference has been released.
// Creates a new FSCursor at the new location.
func (h *FSHandle) Lookup(ctx context.Context, name string) (*FSHandle, error) {
	return h.i().lookup(ctx, name)
}

// LookupPath looks up a path and returns the last FSHandle that was traversed.
//
// Use empty string "" or / for the root.
// Returns ErrNotExist if the entry was not found.
// Returns ErrReleased if the handle has been released.
//
// ErrNotExist is returned if not found and the FSHandle of the parent of the
// element, and the subset of pathParts that is the path to the returned node.
//
// Check if fsHandle is not nil and release it even if an error is returned.
func (h *FSHandle) LookupPath(ctx context.Context, filePath string) (*FSHandle, []string, error) {
	// ignore absolute paths: treat them as relative to ./
	pathParts, err := CleanSplitValidateRelativePath(filePath)
	if err != nil {
		return nil, nil, &fs.PathError{
			Op:   "lookup",
			Path: filePath,
			Err:  err,
		}
	}

	return h.LookupPathPts(ctx, pathParts)
}

// LookupPathHandles recursively traverses the path, returning a handle pointing
// to the target. Returns the subset of filePath that was traversed.
//
// Use empty string "" or / for the root.
// Returns ErrNotExist if the entry was not found.
// Returns ErrReleased if the handle has been released.
//
// ErrNotExist is returned if not found and the FSHandle of the parent of the
// element, and the subset of pathParts that is the path to the returned node.
//
// Check if fsHandle is not nil and release it even if an error is returned.
func (h *FSHandle) LookupPathHandles(ctx context.Context, filePath string) ([]*FSHandle, []string, error) {
	// ignore absolute paths: treat them as relative to ./
	pathParts, err := CleanSplitValidateRelativePath(filePath)
	if err != nil {
		return nil, nil, &fs.PathError{
			Op:   "lookup",
			Path: filePath,
			Err:  err,
		}
	}

	return h.LookupPathPtsHandles(ctx, pathParts)
}

// LookupPathPts looks up a path and returns the subset of pathParts that were
// traversed.
//
// All handles in the path except for the returned one are released. The caller
// only needs to release the returned handle, if any.
//
// ErrNotExist is returned if not found and the FSHandle of the parent of the
// element, and the subset of pathParts that is the path to the returned node.
//
// Check if fsHandle is not nil and release it even if an error is returned.
func (h *FSHandle) LookupPathPts(ctx context.Context, pathParts []string) (*FSHandle, []string, error) {
	outHandle, err := h.Clone(ctx)
	if err != nil {
		return nil, nil, err
	}
	for i, pathPart := range pathParts {
		// these should not be given but handle it anyway
		if pathPart == "." || pathPart == "" {
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

// LookupPathPtsHandles looks up a path and returns the subset of pathParts that
// were traversed.
//
// Returns the FSHandles for each part of pathParts plus index 0 for a clone of h.
//
// Unlike LookupPathPts, the handles in the list are not released, and the
// caller is therefore responsible for releasing all of the handles when done.
//
// ErrNotExist is returned if not found and the FSHandle of the parent of the
// element, and the subset of pathParts that is the path to the returned node.
// Check if handles is empty and release them even if an error is returned.
func (h *FSHandle) LookupPathPtsHandles(ctx context.Context, pathParts []string) ([]*FSHandle, []string, error) {
	currHandle, err := h.Clone(ctx)
	if err != nil {
		return nil, nil, err
	}

	handles := make([]*FSHandle, 1, len(pathParts)+1)
	handles[0] = currHandle

	for i, pathPart := range pathParts {
		// these should not be given but handle it anyway
		if pathPart == "." || pathPart == "" {
			continue
		}

		nextHandle, err := currHandle.Lookup(ctx, pathPart)
		if err != nil {
			if nextHandle != nil {
				nextHandle.Release()
			}
			return handles, pathParts[:i], err
		}

		currHandle = nextHandle
		handles = append(handles, nextHandle)
	}

	return handles, pathParts, nil
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
	return h.i().accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		return ops.Mknod(ctx, checkExist, names, nodeType, permissions, ts)
	})
}

// MkdirAllPath creates a directory named path, along with any necessary parents,
// and returns nil, or else returns an error. The permission bits perm are used
// for all directories that MkdirAllPath creates. If path is/ already a directory,
// MkdirAllPath does nothing and returns nil.
func (h *FSHandle) MkdirAllPath(ctx context.Context, filepath string, perm fs.FileMode, ts time.Time) error {
	if filepath == "" || filepath == "." {
		return nil
	}

	// ignore absolute paths: treat them as relative to ./
	dirPath, _ := SplitPath(filepath)
	return h.MkdirAll(ctx, dirPath, perm, ts)
}

// MkdirAll creates a directory named path, along with any necessary parents,
// and returns nil, or else returns an error. The permission bits perm are used
// for all directories that MkdirAll creates. If path is/ already a directory,
// MkdirAll does nothing and returns nil.
func (h *FSHandle) MkdirAll(ctx context.Context, dirPath []string, perm fs.FileMode, ts time.Time) error {
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

// MkdirLookup performs a lookup for a directory in the handle, creates it if it doesn't exist,
// then looks up again and returns the new handle.
func (h *FSHandle) MkdirLookup(ctx context.Context, name string, perm fs.FileMode, ts time.Time) (*FSHandle, error) {
	dir, err := h.Lookup(ctx, name)
	if err == unixfs_errors.ErrNotExist {
		// Create directory
		err = h.Mknod(ctx, false, []string{name}, NewFSCursorNodeType_Dir(), perm, ts)
		if err != nil {
			return nil, err
		}
		// Lookup again
		dir, err = h.Lookup(ctx, name)
	}
	if err != nil {
		return nil, err
	}

	// Check if it's a directory
	nt, err := dir.GetNodeType(ctx)
	if err != nil {
		dir.Release()
		return nil, err
	}
	if !nt.GetIsDirectory() {
		dir.Release()
		return nil, unixfs_errors.ErrNotDirectory
	}

	return dir, nil
}

// MkdirAllLookup creates a directory named path, along with any necessary parents,
// and returns the handle to the last created or existing directory, or else returns an error.
// The permission bits perm are used for all directories that MkdirAllLookup creates.
// If path is already a directory, MkdirAllLookup returns the handle to that directory.
func (h *FSHandle) MkdirAllLookup(ctx context.Context, dirPath []string, perm fs.FileMode, ts time.Time) (*FSHandle, error) {
	currHandle, err := h.Clone(ctx)
	if err != nil {
		return nil, err
	}
	if len(dirPath) == 0 {
		return currHandle, nil
	}

	for _, pname := range dirPath {
		if pname == "." {
			continue
		}
		newHandle, err := currHandle.MkdirLookup(ctx, pname, perm, ts)
		currHandle.Release()
		if err != nil {
			return nil, err
		}
		currHandle = newHandle
	}

	return currHandle, nil
}

// MkdirAllPathLookup is similar to MkdirAllLookup but takes a string path instead of a slice of path components.
func (h *FSHandle) MkdirAllPathLookup(ctx context.Context, filepath string, perm fs.FileMode, ts time.Time) (*FSHandle, error) {
	if filepath == "" || filepath == "." {
		return h.Clone(ctx)
	}

	// ignore absolute paths: treat them as relative to ./
	dirPath, _ := SplitPath(filepath)
	return h.MkdirAllLookup(ctx, dirPath, perm, ts)
}

// Symlink creates a symbolic link from a location to a path.
func (h *FSHandle) Symlink(ctx context.Context, checkExist bool, name string, target []string, targetIsAbsolute bool, ts time.Time) error {
	if len(name) == 0 || len(target) == 0 {
		return unixfs_errors.ErrEmptyPath
	}
	return h.i().accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		return ops.Symlink(ctx, checkExist, name, target, targetIsAbsolute, ts)
	})
}

// Readlink reads a symbolic link contents.
// If name is empty, reads the link at the FSHandle.
// Returns ErrNotSymlink if not a symbolic link.
// Returns the path, if the symlink is absolute, and any error.
func (h *FSHandle) Readlink(ctx context.Context, name string) ([]string, bool, error) {
	handle := h
	if len(name) != 0 {
		var err error
		handle, err = h.Lookup(ctx, name)
		if err != nil {
			return nil, false, err
		}
		defer handle.Release()
	}

	var link []string
	var isAbs bool
	err := handle.i().accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		var err error
		link, isAbs, err = ops.Readlink(ctx, name)
		return err
	})
	return link, isAbs, err
}

// Copy recursively copies a location to a destination, overwriting destination.
//
// The source and destination must be from the same inode tree.
func (h *FSHandle) Copy(ctx context.Context, dest *FSHandle, destName string, ts time.Time) error {
	if h == nil || dest == nil || dest.CheckReleased() || h.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if h == dest {
		// TODO: copy to self?
		return nil
	}

	// access source inode
	return h.i().accessInode(ctx, func(_ FSCursor, srcOps FSCursorOps) error {
		// access destination inode
		return dest.i().accessInode(ctx, func(_ FSCursor, destOps FSCursorOps) error {
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
			/*
				if le := h.i().f.le; le != nil {
					le.Warnf("TODO: cross-fs copy between locations: %#v -> %#v", srcOps, destOps)
				}
			*/
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

	// attempt to lock src and destination
	var srcLoc, destParent *fsInode
	var relSrcLoc, relDestParent func()
	var err error
	lockedNodes := make(map[*fsInode]func())
	relLockedNodes := func() {
		for i, rel := range lockedNodes {
			rel()
			delete(lockedNodes, i)
		}
	}
	defer relLockedNodes()
	for {
		if err := ctx.Err(); err != nil {
			return context.Canceled
		}

		srcLoc, destParent = h.i(), dest.i()
		// check if srcLoc is destParent
		if srcLoc == destParent {
			return unixfs_errors.ErrMoveToSelf
		}
		// check if srcLoc is a parent of destParent
		for nn := destParent.parent; nn != nil; nn = nn.parent {
			if nn == srcLoc {
				return unixfs_errors.ErrMoveToSelf
			}
		}

		// to prevent deadlock, if we can't lock, unlock both and try locking the opposite order
		relSrcLoc, err = srcLoc.rmtx.Lock(ctx, true)
		if err != nil {
			return err
		}

		var locked bool
		relDestParent, locked = destParent.rmtx.TryLock(true)
		if !locked {
			relSrcLoc()
			relDestParent, err = destParent.rmtx.Lock(ctx, true)
			if err != nil {
				return err
			}

			relSrcLoc, locked = srcLoc.rmtx.TryLock(true)
			if !locked {
				relDestParent()
				continue
			}
		}

		// check that both are not released
		if _, err := srcLoc.checkReleasedWithErr(); err != nil {
			relSrcLoc()
			relDestParent()
			return err
		}
		if _, err := destParent.checkReleasedWithErr(); err != nil {
			relDestParent()
			relSrcLoc()
			return err
		}

		// if either of them are currently resolving fsOps, wait
		srcLocWait := srcLoc.fsWait
		if srcLocWait != nil {
			select {
			case <-srcLocWait:
				srcLocWait, srcLoc.fsWait = nil, nil
			default:
			}
		}

		dstLocWait := destParent.fsWait
		if dstLocWait != nil {
			select {
			case <-dstLocWait:
				dstLocWait, destParent.fsWait = nil, nil
			default:
			}
		}

		if srcLocWait != nil {
			relSrcLoc()
			relDestParent()
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-srcLocWait:
				continue
			}
		}

		if dstLocWait != nil {
			relDestParent()
			relSrcLoc()
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-dstLocWait:
				continue
			}
		}

		// resolve fsOps for src and dest
		fsOpsSrc := srcLoc.fsOps
		if fsOpsSrc == nil || fsOpsSrc.CheckReleased() {
			// release the destination loc for now
			relDestParent()
			// we will perform the lookup
			fsWait := make(chan struct{})
			srcLoc.fsWait = fsWait
			// expects mtx to be locked on entry & released on exit.
			srcLoc.resolveOpsRoutineLocked(ctx, fsWait, relSrcLoc)
			continue
		}

		fsOpsDest := destParent.fsOps
		if fsOpsDest == nil || fsOpsSrc.CheckReleased() {
			// release the src loc for now
			relSrcLoc()
			// we will perform the lookup
			fsWait := make(chan struct{})
			destParent.fsWait = fsWait
			// expects mtx to be locked on entry & released on exit.
			destParent.resolveOpsRoutineLocked(ctx, fsWait, relDestParent)
			continue
		}

		// srcLoc and destParent are locked
		lockedNodes[srcLoc] = relSrcLoc
		lockedNodes[destParent] = relDestParent

		// lock their children as well
		// note: we always lock in parent -> child order
		nodStk := []*fsInode{srcLoc, destParent}
		for len(nodStk) != 0 {
			// pop 1 from nodStk
			nod := nodStk[len(nodStk)-1]
			nodStk[len(nodStk)-1] = nil
			nodStk = nodStk[:len(nodStk)-1]

			for _, nodChild := range nod.children {
				if _, ok := lockedNodes[nodChild]; ok {
					continue
				}

				rel, err := nodChild.rmtx.Lock(ctx, true)
				if err != nil {
					return err
				}

				// ignore if released or if no refs + children
				if nodChild.checkReleased() || nodChild.releaseIfNecessaryLocked() {
					rel()
					continue
				}

				lockedNodes[nodChild] = rel
				nodStk = append(nodStk, nodChild)
			}
		}

		// if the nodes were released while we were doing the above, retry.
		if fsOpsSrc.CheckReleased() || fsOpsDest.CheckReleased() {
			relLockedNodes()
			continue
		}

		// Attempt to perform optimized move from src -> dest.
		done, err := fsOpsSrc.MoveTo(ctx, fsOpsDest, destName, ts)
		if err != nil {
			return err
		}
		if done {
			break
		}

		// Attempt to perform optimized move from dest <- src.
		done, err = fsOpsDest.MoveFrom(ctx, destName, fsOpsSrc, ts)
		if err != nil {
			return err
		}
		if done {
			break
		}

		// No optimized path exists, do it the slow way.
		// TODO: recursive move
		/*
			if le := h.i().f.le; le != nil {
				le.Warnf("TODO: cross-fs rename between locations: %#v -> %#v", srcOps, destOps)
			}
		*/
		return unixfs_errors.ErrCrossFsRename
	}

	// successful rename: the source location has likely already been released,
	// and the destination location should have had a change callback called.

	// remove the source inode from the parent children list
	if parent := srcLoc.parent; parent != nil {
		oldChild, oldChildIdx := parent.findChildInode(srcLoc.name, false)
		if oldChild != nil {
			// remove the old child from the parent
			parent.removeChildInodeAtIdx(oldChildIdx)
			// release the old child if it's not srcLoc (unlikely)
			if oldChild != srcLoc {
				oldChild.releaseWithChildrenLocked(nil)
			}
		}
	}

	// destination parent was released; nothing further we can do from here.
	if destParent.checkReleased() {
		destParent.releaseLocked(unixfs_errors.ErrReleased)
		return unixfs_errors.ErrReleased
	}

	// lookup or create the destination inode location
	destLoc, destLocIdx := destParent.findChildInode(destName, true)
	if destLoc == nil {
		// child inode not found, insert at insertidx.
		destLoc = newFsInode(destParent, destName, nil)
		destParent.children = slices.Insert(destParent.children, destLocIdx, destLoc)
	}

	// merge srcLoc -> destLoc: moving refs and children
	destLoc.mergeWithNodeLocked(srcLoc, unixfs_errors.ErrReleased)

	// recursively release / clear all fs cursors and ops for destLoc
	destLoc.clearCursorsWithChildrenLocked()

	// done
	return nil
}

// Remove removes entries from a directory.
func (h *FSHandle) Remove(ctx context.Context, names []string, ts time.Time) error {
	if len(names) == 0 {
		return nil
	}
	return h.i().accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		return ops.Remove(ctx, names, ts)
	})
}

// Clone makes a copy of the FSHandle.
func (h *FSHandle) Clone(ctx context.Context) (*FSHandle, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	inode := h.i()
	rel, err := inode.rmtx.Lock(ctx, true)
	if err != nil {
		return nil, err
	}
	defer rel()

	return inode.addReferenceLocked(true)
}

// Release releases the FSHandle.
func (h *FSHandle) Release() {
	if h.isReleased.Swap(true) {
		// already released
		return
	}
	inode := h.i()
	rel, err := inode.rmtx.Lock(context.Background(), true)
	if err == nil {
		inode.removeRefLocked(h)
		rel()
	}
	for {
		relCb := h.relCbs.Pop()
		if relCb == nil {
			break
		}
		relCb()
	}
}

// i returns the inode.
func (h *FSHandle) i() *fsInode {
	return h.inode.Load()
}
