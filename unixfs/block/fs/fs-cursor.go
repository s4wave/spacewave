package unixfs_block_fs

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// FSCursor implements a FSCursor attached to FS.
type FSCursor struct {
	// isReleased is an atomic int indicating if this cursor is released
	isReleased uint32
	// changeNum is an atomic int indicating the number of change events
	changeNum uint32
	// fs is the filesystem
	fs *FS
	// depth is the inode depth relative to root of fs
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
	// note: only call when fs.rmtx is locked
	cbs unixfs.FSCursorChangeCbSlice
	// fsCursorOps is the filesystem ops object.
	// nil until resolved
	fsCursorOps *FSCursorOps
	// path is the path to this cursor
	// nil until resolved
	path []string
}

// newFSCursor constructs a new FSCursor with details.
// expects fs.rmtx to be locked.
// fsTree can be nil to defer looking up from parent until later
// if fsTree is not nil, constructs the fsOps immediately
// btx can be nil
// returns nil if the parent was already released.
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
		if !parent.lockedAddChangeCb(c.handleParentChanged) {
			return nil
		}
	}
	if fsTree != nil {
		c.fsCursorOps = newFSCursorOps(c, fsTree, btx)
	}
	return c
}

// CheckReleased checks if the fscursor is released without locking anything.
func (f *FSCursor) CheckReleased() bool {
	return atomic.LoadUint32(&f.isReleased) == 1
}

// GetPath resolves and returns the path to this cursor.
// Note: do not edit the returned slice!
func (f *FSCursor) GetPath() []string {
	if f.parent == nil {
		return nil
	}

	f.fs.rmtx.Lock()
	defer f.fs.rmtx.Unlock()

	// build the path
	return f.getOrBuildPathLocked()
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

	f.fs.rmtx.Lock()
	cbAdded := f.lockedAddChangeCb(cb)
	if !cbAdded {
		// call cb with released right away
		_ = cb(&unixfs.FSCursorChange{Cursor: f, Released: true})
	}
	f.fs.rmtx.Unlock()
}

// GetProxyCursor returns a FSCursor to replace this one, if necessary.
func (f *FSCursor) GetProxyCursor(ctx context.Context) (unixfs.FSCursor, error) {
	// TODO: Check if this path prefix is "mounted" on the FS.
	return nil, nil
}

// GetFSCursorOps returns the interface implementing FSCursorOps.
// Called after AddChangeCb and only if GetProxyCursor returns nil, nil.
// Return nil, nil to indicate this position is null (nothing here).
// Return nil, ErrReleased to indicate this FSCursor was released.
func (f *FSCursor) GetFSCursorOps(ctx context.Context) (unixfs.FSCursorOps, error) {
	if f.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}

	f.fs.rmtx.Lock()
	defer f.fs.rmtx.Unlock()
	if f.fsCursorOps != nil {
		if f.fsCursorOps.CheckReleased() {
			f.fsCursorOps.release()
			f.fsCursorOps = nil
		} else {
			return f.fsCursorOps, nil
		}
	}

	if err := f.resolveFsCursorOps(); err != nil {
		// error resolving, release the cursor
		f.lockedRelease()
		return nil, err
	}

	v := f.fsCursorOps
	if v == nil {
		// nil after resolving, something must have gone wrong.
		f.lockedRelease()
		return nil, unixfs_errors.ErrReleased
	}
	return v, nil
}

// buildChildCursor locks fs.rmtx and builds a child cursor with a name
// if the dirent is set, builds the ops immediately with the dirent
func (f *FSCursor) buildChildCursor(name string, dirent *unixfs_block.Dirent) (unixfs.FSCursor, error) {
	f.fs.rmtx.Lock()
	defer f.fs.rmtx.Unlock()

	var ftree *unixfs_block.FSTree
	var btx *block.Transaction
	if dirent != nil { // && !dirent.GetNodeRef().GetEmpty() {
		var bcs *block.Cursor
		btx, bcs = f.fs.rootCursor.BuildTransactionAtRef(nil, dirent.GetNodeRef())
		var err error
		ftree, err = unixfs_block.NewFSTree(bcs, dirent.GetNodeType())
		if err != nil {
			// ignore error here, defer to later.
			ftree = nil
		}
	}

	// if ftree is nil, lookup will be deferred.
	return newFSCursor(f.fs, f, name, ftree, btx), nil
}

// lockedAddChangeCb calls AddChangeCb when rmtx is locked by caller.
// returns if the callback was added or not.
// the return value is !f.released
func (f *FSCursor) lockedAddChangeCb(cb unixfs.FSCursorChangeCb) bool {
	released := f.CheckReleased()
	if !released {
		f.cbs = append(f.cbs, cb)
	}
	return !released
}

// handleParentChanged handles the changed callback from the parent.
func (f *FSCursor) handleParentChanged(ch *unixfs.FSCursorChange) bool {
	// if cursor released, do nothing.
	if f.CheckReleased() {
		return false
	}

	// If parent released: release cursor.
	if ch != nil && ch.Released {
		// separate goroutine to avoid mutex contention
		go f.Release()
		return false
	}

	// Parent changed, trigger checking if anything changed with this node.
	id := atomic.AddUint32(&f.changeNum, 1)
	go f.checkForChangesFromParent(id)
	return true
}

// checkForChangesFromParent is a goroutine which checks if the fscursor changed.
func (f *FSCursor) checkForChangesFromParent(chId uint32) {
	// quick check if the root ctx is canceled.
	select {
	case <-f.fs.ctx.Done():
		f.Release()
		return
	default:
	}

	f.fs.rmtx.Lock()

	// check if some newer goroutine was started already to do this
	if atomic.LoadUint32(&f.changeNum) != chId {
		f.fs.rmtx.Unlock()
		return
	}

	// check if the existing ops was released already
	if f.fsCursorOps != nil && f.fsCursorOps.CheckReleased() {
		f.fsCursorOps.release()
		f.fsCursorOps = nil
	}

	// if we have not yet resolved the fsOps, ignore.
	if f.fsCursorOps == nil || f.parent == nil {
		f.fs.rmtx.Unlock()
		return
	}

	// this is a race condition check, unlikely.
	if f.parent.CheckReleased() {
		f.fs.rmtx.Unlock()
		// parent released, release this.
		f.Release()
		return
	}

	// lookup this node again from parent
	if err := f.parent.resolveFsCursorOps(); err != nil || f.parent.fsCursorOps == nil {
		// error resolving parent cursor ops. release this.
		f.fs.rmtx.Unlock()
		// parent released, release this.
		f.Release()
		return
	}

	dirEnt, err := f.parent.fsCursorOps.fsTree.Lookup(f.name)
	if err == nil && dirEnt == nil {
		err = unixfs_errors.ErrNotExist
	}
	if err != nil {
		// error: clear / release the cursor.
		// doesn't matter what kind of error
		f.fs.rmtx.Unlock()
		f.Release()
		if err != context.Canceled && err != unixfs_errors.ErrNotExist && f.fs.writer != nil {
			f.fs.writer.FilesystemError(err)
		}
		return
	}

	// check if it matches
	ops := f.fsCursorOps
	if ops.fsTree.GetCursorRef().EqualsRef(dirEnt.GetNodeRef()) {
		// identical block ref, no changes.
		f.fs.rmtx.Unlock()
		return
	}

	// changed: trigger callback for all subscribers and clear cache.

	// NOTE: this can be optimized by detecting changes in file contents by
	// range and invalidating those with the change callback. for now, just
	// invalidate the cursor / cache entirely and force re-check on read.
	if f.fsCursorOps != nil {
		f.fsCursorOps.release()
		f.fsCursorOps = nil
	}

	if len(f.cbs) != 0 {
		f.cbs = f.cbs.CallCbs(&unixfs.FSCursorChange{Cursor: f})
	}
	f.fs.rmtx.Unlock()
}

// resolveFsCursorOps builds the fsCursorOps field.
// caller must lock rmtx
// if this returns an error, most likely FSCursor should be released.
func (f *FSCursor) resolveFsCursorOps() error {
	if f.fsCursorOps != nil {
		if f.fsCursorOps.CheckReleased() {
			f.fsCursorOps.release()
			f.fsCursorOps = nil
		} else {
			return nil
		}
	}

	if f.parent == nil {
		// root node: build from root fs
		ftree, _, btx, err := f.fs.buildRootTx()
		if err != nil {
			return err
		}
		f.fsCursorOps = newFSCursorOps(f, ftree, btx)
		return nil
	}

	// get from parent
	if err := f.parent.resolveFsCursorOps(); err != nil {
		return err
	}

	// lookup our dirent
	dirEnt, err := f.parent.fsCursorOps.fsTree.Lookup(f.name)
	if err == nil {
		if !dirEnt.GetNodeRef().GetEmpty() {
			err = dirEnt.GetNodeRef().Validate()
		}
	}
	if err != nil {
		if err != context.Canceled && err != unixfs_errors.ErrNotExist && f.fs.writer != nil {
			f.fs.writer.FilesystemError(err)
		}
		return err
	}

	// build initial fsops
	btx, bcs := f.fs.rootCursor.BuildTransactionAtRef(nil, dirEnt.GetNodeRef())
	ftree, err := unixfs_block.NewFSTree(bcs, dirEnt.GetNodeType())
	if err != nil {
		if err != context.Canceled && f.fs.writer != nil {
			f.fs.writer.FilesystemError(err)
		}
		return err
	}
	f.fsCursorOps = newFSCursorOps(f, ftree, btx)
	return nil
}

// getOrBuildPathLocked gets or builds the path to this FSCursor.
// caller must lock fs.mtx
func (f *FSCursor) getOrBuildPathLocked() []string {
	if f.parent == nil || len(f.path) != 0 {
		return f.path
	}

	npath := make([]string, 0, f.depth)
	stk := make([]*FSCursor, 1, f.depth)
	stk[0] = f
	for {
		nparent := stk[len(stk)-1].parent
		if nparent == nil {
			break
		}
		if len(nparent.path) != 0 {
			npath = append(npath, nparent.path...)
			break
		}
		stk = append(stk, nparent)
	}
	for i := len(stk) - 1; i >= 0; i-- {
		nname := stk[i].name
		if nname != "" {
			npath = append(npath, nname)
			// note: we already checked that path was nil above
			stk[i].path = npath
		}
	}
	return npath
}

// Release releases the filesystem cursor.
// note: locks rmtx. must NOT be locked when calling
func (f *FSCursor) Release() {
	if f.CheckReleased() {
		return
	}

	f.fs.rmtx.Lock()
	f.lockedRelease()
	f.fs.rmtx.Unlock()
}

// lockedRelease releases the FSCursor with fs.rmtx locked
func (f *FSCursor) lockedRelease() {
	if atomic.SwapUint32(&f.isReleased, 1) == 1 {
		// already released
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

// _ is a type assertion
var _ unixfs.FSCursor = ((*FSCursor)(nil))
