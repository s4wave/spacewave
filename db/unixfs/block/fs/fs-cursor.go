package unixfs_block_fs

import (
	"context"
	"slices"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// FSCursor implements a FSCursor attached to FS.
type FSCursor struct {
	// isReleased indicates if this is released.
	isReleased atomic.Bool
	// fs is the filesystem root
	fs *FS
	// depth is the inode depth relative to root of fs
	// note: not always accurate, used as an estimate for alloc slices
	// immutable
	depth uint
	// name is the name of this node
	// immutable
	name string
	// parent is the parent FSCursor or nil if none
	// immutable
	parent *FSCursor

	// all below fields are guarded by fs.rmtx

	// cbs is the list of change callbacks
	// note: only call while fs.rmtx is locked!
	cbs unixfs.FSCursorChangeCbSlice
	// fsCursorOps is the filesystem ops object.
	// nil until resolved
	fsCursorOps *FSCursorOps
	// path is the path to this cursor
	// nil unless parent != nil
	// nil until resolved
	path atomic.Pointer[[]string]
}

// newFSCursor constructs a new FSCursor with details.
// expects fs.rmtx to be locked.
// fsTree can be nil to defer looking up from parent until later
// if fsTree is not nil, constructs the fsOps immediately
// btx can be nil
func newFSCursor(
	fs *FS,
	parent *FSCursor,
	name string,
	fsTree *unixfs_block.FSTree,
	btx *block.Transaction,
) *FSCursor {
	var depth uint
	if parent != nil {
		depth = parent.depth + 1
	}
	c := &FSCursor{
		fs:     fs,
		depth:  depth,
		parent: parent,
		name:   name,
	}
	if parent != nil {
		if !parent.addChangeCbLocked(c.handleParentChangedLocked) {
			// mark as released and stop here if parent is released.
			c.releaseLocked()
			return c
		}
	}
	if fsTree != nil {
		c.fsCursorOps = newFSCursorOps(c, fsTree, btx)
	}
	return c
}

// CheckReleased checks if the fscursor is released without locking anything.
func (f *FSCursor) CheckReleased() bool {
	return f.isReleased.Load()
}

// GetPath resolves and returns the path to this cursor.
// Note: do not edit the returned slice!
func (f *FSCursor) GetPath(ctx context.Context) ([]string, error) {
	if f.parent == nil {
		return nil, nil
	}

	// build the path
	return f.getOrBuildPath(ctx, false)
}

// AddChangeCb adds a change callback to detect when the cursor has changed.
// This will be called only if GetProxyCursor returns nil, nil.
//
// cb will always be called while f.fs.rmtx is locked
// cb must not block, and should be called when cursor changes / is released
// cb will be called immediately (same call tree) if already released.
func (f *FSCursor) AddChangeCb(cb unixfs.FSCursorChangeCb) {
	if cb == nil {
		return
	}

	rel, err := f.fs.rmtx.Lock(context.Background(), true)
	if err != nil {
		return
	}
	defer rel()

	cbAdded := f.addChangeCbLocked(cb)
	if !cbAdded {
		// call cb with released right away
		_ = cb(&unixfs.FSCursorChange{Cursor: f, Released: true})
	}
}

// GetProxyCursor returns a FSCursor to replace this one, if necessary.
func (f *FSCursor) GetProxyCursor(ctx context.Context) (unixfs.FSCursor, error) {
	return nil, nil
}

// GetCursorOps returns the interface implementing FSCursorOps.
// Called after AddChangeCb and only if GetProxyCursor returns nil, nil.
// Return nil, nil to indicate this position is null (nothing here).
// Return nil, ErrReleased to indicate this FSCursor was released.
func (f *FSCursor) GetCursorOps(ctx context.Context) (unixfs.FSCursorOps, error) {
	if f.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}

	rel, err := f.fs.rmtx.Lock(ctx, true)
	if err != nil {
		return nil, err
	}
	defer rel()

	if f.fsCursorOps != nil {
		if f.fsCursorOps.CheckReleased() {
			f.fsCursorOps = nil
		} else {
			return f.fsCursorOps, nil
		}
	}

	if err := f.resolveFsCursorOpsLocked(); err != nil {
		// error resolving, release the cursor
		f.releaseLocked()
		return nil, err
	}

	v := f.fsCursorOps
	if v == nil {
		// nil after resolving, something must have gone wrong.
		f.releaseLocked()
		return nil, unixfs_errors.ErrReleased
	}

	return v, nil
}

// buildChildCursor locks fs.rmtx and builds a child cursor with a name
// if the dirent is set, builds the ops immediately with the dirent
// if dirent and childCs are set, detaches childCs to use for the new child.
// expects f.mtx to be locked
func (f *FSCursor) buildChildCursor(ctx context.Context, name string, dirent *unixfs_block.Dirent, childCs *block.Cursor) (unixfs.FSCursor, error) {
	rel, err := f.fs.rmtx.Lock(ctx, true)
	if err != nil {
		return nil, err
	}
	defer rel()

	var ftree *unixfs_block.FSTree
	var btx *block.Transaction
	if dirent != nil { // && !dirent.GetNodeRef().GetEmpty() {
		var bcs *block.Cursor

		if childCs != nil {
			bcs = childCs.DetachRecursive(true, true, false)
			btx = bcs.GetTransaction()
		} else {
			// build new tx using ref
			btx, bcs = f.fs.bls.BuildTransactionAtRef(nil, dirent.GetNodeRef())
		}

		var err error
		ftree, err = unixfs_block.NewFSTree(f.fs.ctx, bcs, dirent.GetNodeType())
		if err != nil {
			// ignore error here, defer to later.
			ftree = nil
		}
	}

	// if ftree is nil, lookup will be deferred.
	return newFSCursor(f.fs, f, name, ftree, btx), nil
}

// addChangeCbLocked calls AddChangeCb when rmtx is locked by caller.
// returns if the callback was added or not.
// the return value is !f.released
func (f *FSCursor) addChangeCbLocked(cb unixfs.FSCursorChangeCb) bool {
	released := f.CheckReleased()
	if !released {
		f.cbs = append(f.cbs, cb)
	}
	return !released
}

// resolveFsCursorOpsLocked builds the fsCursorOps field.
// caller must lock rmtx
// if this returns an error, most likely FSCursor should be released.
func (f *FSCursor) resolveFsCursorOpsLocked() error {
	if f.fsCursorOps != nil {
		if f.fsCursorOps.CheckReleased() {
			f.fsCursorOps = nil
		} else {
			return nil
		}
	}

	if f.parent == nil {
		// root node: build from root fs
		ftree, _, btx, err := f.fs.buildRootTxLocked()
		if err != nil {
			return err
		}
		f.fsCursorOps = newFSCursorOps(f, ftree, btx)
		return nil
	}

	// get from parent
	if err := f.parent.resolveFsCursorOpsLocked(); err != nil {
		return err
	}

	// lookup our dirent
	dirEnt, err := f.parent.fsCursorOps.fsTree.Lookup(f.name)
	if err == nil {
		// allow empty node ref
		err = dirEnt.GetNodeRef().Validate(true)
	}
	if err != nil {
		if err != context.Canceled && err != unixfs_errors.ErrNotExist && f.fs.writer != nil {
			f.fs.writer.FilesystemError(err)
		}
		return err
	}

	// build initial fsops
	btx, bcs := f.fs.bls.BuildTransactionAtRef(nil, dirEnt.GetNodeRef())
	ftree, err := unixfs_block.NewFSTree(f.fs.ctx, bcs, dirEnt.GetNodeType())
	if err != nil {
		if err != context.Canceled && f.fs.writer != nil {
			f.fs.writer.FilesystemError(err)
		}
		return err
	}
	f.fsCursorOps = newFSCursorOps(f, ftree, btx)

	return nil
}

// getOrBuildPath gets or builds the path to this FSCursor.
// if fs.mtx is locked, set locked=true
func (f *FSCursor) getOrBuildPath(ctx context.Context, locked bool) ([]string, error) {
	if f.parent == nil {
		return nil, nil
	}
	if fpath := f.path.Load(); fpath != nil {
		return *fpath, nil
	}
	if !locked {
		rel, err := f.fs.rmtx.Lock(ctx, true)
		if err != nil {
			return nil, err
		}
		defer rel()

		if fpath := f.path.Load(); fpath != nil {
			return *fpath, nil
		}
	}

	npath := make([]string, 0, f.depth)
	stk := make([]*FSCursor, 1, f.depth)
	stk[0] = f
	for {
		nparent := stk[len(stk)-1].parent
		if nparent == nil {
			break
		}
		nparentPath := nparent.path.Load()
		if nparentPath != nil {
			npath = append(npath, (*nparentPath)...)
			break
		}
		stk = append(stk, nparent)
	}
	for _, v := range slices.Backward(stk) {
		nname := v.name
		if nname != "" {
			npath = append(npath, nname)
			npathAtI := npath
			v.path.Store(&npathAtI)
		}
	}
	return npath, nil
}

// Release releases the filesystem cursor.
// note: locks rmtx. must NOT be locked when calling
func (f *FSCursor) Release() {
	if f.CheckReleased() {
		return
	}

	rel, err := f.fs.rmtx.Lock(context.Background(), true)
	if err != nil {
		return
	}
	defer rel()

	f.releaseLocked()
}

// releaseLocked releases the FSCursor with fs.rmtx locked
func (f *FSCursor) releaseLocked() {
	if f.isReleased.Swap(true) {
		return
	}
	cbs := f.cbs
	f.cbs = nil
	if f.fsCursorOps != nil {
		f.fsCursorOps.release()
		f.fsCursorOps = nil
	}
	_ = cbs.CallCbs(&unixfs.FSCursorChange{Cursor: f, Released: true})
}

// handleParentChangedLocked handles the changed callback from the parent.
// we hold the rmtx write lock.
func (f *FSCursor) handleParentChangedLocked(ch *unixfs.FSCursorChange) bool {
	// if cursor released, do nothing.
	if f.CheckReleased() {
		return false
	}

	// If parent released: release cursor.
	if ch != nil && ch.Released {
		f.releaseLocked()
		return false
	}

	// Ignore other change events
	return true
}

// _ is a type assertion
var _ unixfs.FSCursor = ((*FSCursor)(nil))
